package rolling

import (
	"fmt"
	"testing"
	"time"

	kapi "k8s.io/kubernetes/pkg/api"
	ktestclient "k8s.io/kubernetes/pkg/client/testclient"
	"k8s.io/kubernetes/pkg/kubectl"
	"k8s.io/kubernetes/pkg/runtime"

	api "github.com/openshift/origin/pkg/api/latest"
	deployapi "github.com/openshift/origin/pkg/deploy/api"
	deploytest "github.com/openshift/origin/pkg/deploy/api/test"
	strat "github.com/openshift/origin/pkg/deploy/strategy"
	deployutil "github.com/openshift/origin/pkg/deploy/util"
)

func TestRolling_deployInitial(t *testing.T) {
	initialStrategyInvoked := false

	strategy := &RollingDeploymentStrategy{
		codec:  api.Codec,
		client: ktestclient.NewSimpleFake(),
		initialStrategy: &testStrategy{
			deployFn: func(from *kapi.ReplicationController, to *kapi.ReplicationController, desiredReplicas int, updateAcceptor strat.UpdateAcceptor) error {
				initialStrategyInvoked = true
				return nil
			},
		},
		rollingUpdate: func(config *kubectl.RollingUpdaterConfig) error {
			t.Fatalf("unexpected call to rollingUpdate")
			return nil
		},
		getUpdateAcceptor: getUpdateAcceptor,
	}

	config := deploytest.OkDeploymentConfig(1)
	config.Template.Strategy = deploytest.OkRollingStrategy()
	deployment, _ := deployutil.MakeDeployment(config, kapi.Codec)
	err := strategy.Deploy(nil, deployment, 2)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !initialStrategyInvoked {
		t.Fatalf("expected initial strategy to be invoked")
	}
}

func TestRolling_deployRolling(t *testing.T) {
	latestConfig := deploytest.OkDeploymentConfig(1)
	latestConfig.Template.Strategy = deploytest.OkRollingStrategy()
	latest, _ := deployutil.MakeDeployment(latestConfig, kapi.Codec)
	config := deploytest.OkDeploymentConfig(2)
	config.Template.Strategy = deploytest.OkRollingStrategy()
	deployment, _ := deployutil.MakeDeployment(config, kapi.Codec)

	deployments := map[string]*kapi.ReplicationController{
		latest.Name:     latest,
		deployment.Name: deployment,
	}
	deploymentUpdated := false

	fake := &ktestclient.Fake{}
	fake.ReactFn = func(action ktestclient.Action) (runtime.Object, error) {
		switch a := action.(type) {
		case ktestclient.GetAction:
			return deployments[a.GetName()], nil
		case ktestclient.UpdateAction:
			deploymentUpdated = true
			return a.GetObject().(*kapi.ReplicationController), nil
		}
		return nil, nil
	}

	var rollingConfig *kubectl.RollingUpdaterConfig
	strategy := &RollingDeploymentStrategy{
		codec:  api.Codec,
		client: fake,
		initialStrategy: &testStrategy{
			deployFn: func(from *kapi.ReplicationController, to *kapi.ReplicationController, desiredReplicas int, updateAcceptor strat.UpdateAcceptor) error {
				t.Fatalf("unexpected call to initial strategy")
				return nil
			},
		},
		rollingUpdate: func(config *kubectl.RollingUpdaterConfig) error {
			rollingConfig = config
			return nil
		},
		getUpdateAcceptor: getUpdateAcceptor,
	}

	err := strategy.Deploy(latest, deployment, 2)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if rollingConfig == nil {
		t.Fatalf("expected rolling update to be invoked")
	}

	if e, a := latest, rollingConfig.OldRc; e != a {
		t.Errorf("expected rollingConfig.OldRc %v, got %v", e, a)
	}

	if e, a := deployment, rollingConfig.NewRc; e != a {
		t.Errorf("expected rollingConfig.NewRc %v, got %v", e, a)
	}

	if e, a := 1*time.Second, rollingConfig.Interval; e != a {
		t.Errorf("expected Interval %d, got %d", e, a)
	}

	if e, a := 1*time.Second, rollingConfig.UpdatePeriod; e != a {
		t.Errorf("expected UpdatePeriod %d, got %d", e, a)
	}

	if e, a := 20*time.Second, rollingConfig.Timeout; e != a {
		t.Errorf("expected Timeout %d, got %d", e, a)
	}

	// verify hack
	if e, a := 1, rollingConfig.NewRc.Spec.Replicas; e != a {
		t.Errorf("expected rollingConfig.NewRc.Spec.Replicas %d, got %d", e, a)
	}

	// verify hack
	if !deploymentUpdated {
		t.Errorf("expected deployment to be updated for source annotation")
	}
	sid := fmt.Sprintf("%s:%s", latest.Name, latest.ObjectMeta.UID)
	if e, a := sid, rollingConfig.NewRc.Annotations[sourceIdAnnotation]; e != a {
		t.Errorf("expected sourceIdAnnotation %s, got %s", e, a)
	}
}

func TestRolling_deployRollingHooks(t *testing.T) {
	config := deploytest.OkDeploymentConfig(1)
	config.Template.Strategy = deploytest.OkRollingStrategy()
	latest, _ := deployutil.MakeDeployment(config, kapi.Codec)

	var hookError error

	deployments := map[string]*kapi.ReplicationController{latest.Name: latest}

	fake := &ktestclient.Fake{}
	fake.ReactFn = func(action ktestclient.Action) (runtime.Object, error) {
		switch a := action.(type) {
		case ktestclient.GetAction:
			return deployments[a.GetName()], nil
		case ktestclient.UpdateAction:
			return a.GetObject().(*kapi.ReplicationController), nil
		}
		return nil, nil
	}

	strategy := &RollingDeploymentStrategy{
		codec:  api.Codec,
		client: fake,
		initialStrategy: &testStrategy{
			deployFn: func(from *kapi.ReplicationController, to *kapi.ReplicationController, desiredReplicas int, updateAcceptor strat.UpdateAcceptor) error {
				t.Fatalf("unexpected call to initial strategy")
				return nil
			},
		},
		rollingUpdate: func(config *kubectl.RollingUpdaterConfig) error {
			return nil
		},
		hookExecutor: &hookExecutorImpl{
			executeFunc: func(hook *deployapi.LifecycleHook, deployment *kapi.ReplicationController, label string) error {
				return hookError
			},
		},
		getUpdateAcceptor: getUpdateAcceptor,
	}

	cases := []struct {
		params               *deployapi.RollingDeploymentStrategyParams
		hookShouldFail       bool
		deploymentShouldFail bool
	}{
		{rollingParams(deployapi.LifecycleHookFailurePolicyAbort, ""), true, true},
		{rollingParams(deployapi.LifecycleHookFailurePolicyAbort, ""), false, false},
		{rollingParams("", deployapi.LifecycleHookFailurePolicyAbort), true, false},
		{rollingParams("", deployapi.LifecycleHookFailurePolicyAbort), false, false},
	}

	for _, tc := range cases {
		config := deploytest.OkDeploymentConfig(2)
		config.Template.Strategy.RollingParams = tc.params
		deployment, _ := deployutil.MakeDeployment(config, kapi.Codec)
		deployments[deployment.Name] = deployment
		hookError = nil
		if tc.hookShouldFail {
			hookError = fmt.Errorf("hook failure")
		}
		err := strategy.Deploy(latest, deployment, 2)
		if err != nil && tc.deploymentShouldFail {
			t.Logf("got expected error: %v", err)
		}
		if err == nil && tc.deploymentShouldFail {
			t.Errorf("expected an error for case: %v", tc)
		}
		if err != nil && !tc.deploymentShouldFail {
			t.Errorf("unexpected error for case: %v: %v", tc, err)
		}
	}
}

// TestRolling_deployInitialHooks can go away once the rolling strategy
// supports initial deployments.
func TestRolling_deployInitialHooks(t *testing.T) {
	var hookError error

	strategy := &RollingDeploymentStrategy{
		codec:  api.Codec,
		client: ktestclient.NewSimpleFake(),
		initialStrategy: &testStrategy{
			deployFn: func(from *kapi.ReplicationController, to *kapi.ReplicationController, desiredReplicas int, updateAcceptor strat.UpdateAcceptor) error {
				return nil
			},
		},
		rollingUpdate: func(config *kubectl.RollingUpdaterConfig) error {
			return nil
		},
		hookExecutor: &hookExecutorImpl{
			executeFunc: func(hook *deployapi.LifecycleHook, deployment *kapi.ReplicationController, label string) error {
				return hookError
			},
		},
		getUpdateAcceptor: getUpdateAcceptor,
	}

	cases := []struct {
		params               *deployapi.RollingDeploymentStrategyParams
		hookShouldFail       bool
		deploymentShouldFail bool
	}{
		{rollingParams(deployapi.LifecycleHookFailurePolicyAbort, ""), true, true},
		{rollingParams(deployapi.LifecycleHookFailurePolicyAbort, ""), false, false},
		{rollingParams("", deployapi.LifecycleHookFailurePolicyAbort), true, false},
		{rollingParams("", deployapi.LifecycleHookFailurePolicyAbort), false, false},
	}

	for _, tc := range cases {
		config := deploytest.OkDeploymentConfig(2)
		config.Template.Strategy.RollingParams = tc.params
		deployment, _ := deployutil.MakeDeployment(config, kapi.Codec)
		hookError = nil
		if tc.hookShouldFail {
			hookError = fmt.Errorf("hook failure")
		}
		err := strategy.Deploy(nil, deployment, 2)
		if err != nil && tc.deploymentShouldFail {
			t.Logf("got expected error: %v", err)
		}
		if err == nil && tc.deploymentShouldFail {
			t.Errorf("expected an error for case: %v", tc)
		}
		if err != nil && !tc.deploymentShouldFail {
			t.Errorf("unexpected error for case: %v: %v", tc, err)
		}
	}
}

type testStrategy struct {
	deployFn func(from *kapi.ReplicationController, to *kapi.ReplicationController, desiredReplicas int, updateAcceptor strat.UpdateAcceptor) error
}

func (s *testStrategy) DeployWithAcceptor(from *kapi.ReplicationController, to *kapi.ReplicationController, desiredReplicas int, updateAcceptor strat.UpdateAcceptor) error {
	return s.deployFn(from, to, desiredReplicas, updateAcceptor)
}

func mkintp(i int) *int64 {
	v := int64(i)
	return &v
}

func rollingParams(preFailurePolicy, postFailurePolicy deployapi.LifecycleHookFailurePolicy) *deployapi.RollingDeploymentStrategyParams {
	var pre *deployapi.LifecycleHook
	var post *deployapi.LifecycleHook

	if len(preFailurePolicy) > 0 {
		pre = &deployapi.LifecycleHook{
			FailurePolicy: preFailurePolicy,
			ExecNewPod:    &deployapi.ExecNewPodHook{},
		}
	}
	if len(postFailurePolicy) > 0 {
		post = &deployapi.LifecycleHook{
			FailurePolicy: postFailurePolicy,
			ExecNewPod:    &deployapi.ExecNewPodHook{},
		}
	}
	return &deployapi.RollingDeploymentStrategyParams{
		UpdatePeriodSeconds: mkintp(1),
		IntervalSeconds:     mkintp(1),
		TimeoutSeconds:      mkintp(20),
		Pre:                 pre,
		Post:                post,
	}
}

func getUpdateAcceptor(timeout time.Duration) strat.UpdateAcceptor {
	return &testAcceptor{
		acceptFn: func(deployment *kapi.ReplicationController) error {
			return nil
		},
	}
}

type testAcceptor struct {
	acceptFn func(*kapi.ReplicationController) error
}

func (t *testAcceptor) Accept(deployment *kapi.ReplicationController) error {
	return t.acceptFn(deployment)
}
