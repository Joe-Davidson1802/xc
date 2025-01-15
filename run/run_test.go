package run

import (
	"context"
	"errors"
	"sync"
	"testing"

	"github.com/joerdav/xc/models"
)

type mockScriptRunner struct {
	calls       int
	returns     error
	runnerMutex sync.Mutex
}

func (r *mockScriptRunner) Execute(
	ctx context.Context, text string, env, args []string, dir, logPrefix string,
) error {
	r.runnerMutex.Lock()
	defer r.runnerMutex.Unlock()
	r.calls++
	return r.returns
}

type testCase struct {
	name               string
	tasks              models.Tasks
	taskName           string
	err                error
	expectedRunError   bool
	expectedParseError bool
	expectedTasksRun   int
}

func testCases() []testCase {
	return []testCase{
		{
			name: "given an invalid task should not run command",
			tasks: []models.Task{
				{
					Name: "mytask",
				},
			},
			taskName:         "fake",
			expectedRunError: true,
		},
		{
			name: "given a valid command should run",
			tasks: []models.Task{
				{
					Name:   "mytask",
					Script: "somecmd",
				},
			},
			taskName:         "mytask",
			expectedTasksRun: 1,
		},
		{
			name: "given a circular task should not run",
			tasks: []models.Task{
				{
					Name:      "mytask",
					DependsOn: []string{"mytask2"},
				},
				{
					Name:      "mytask2",
					DependsOn: []string{"mytask"},
				},
			},
			taskName:           "mytask",
			expectedParseError: true,
		},
		{
			name: "given a valid command with deps only should run",
			tasks: []models.Task{
				{
					Name:   "mytask",
					Script: "somecmd",
				},
				{
					Name:      "mytask2",
					DependsOn: []string{"mytask"},
				},
			},
			taskName:         "mytask2",
			expectedTasksRun: 1,
		},
		{
			name: "given first task fails stop",
			tasks: []models.Task{
				{
					Name:   "mytask",
					Script: "somecmd",
				},
				{
					Name:      "mytask2",
					Script:    "somecmd2",
					Dir:       ".",
					DependsOn: []string{"mytask"},
				},
			},
			taskName:         "mytask2",
			err:              errors.New("some error"),
			expectedTasksRun: 1,
			expectedRunError: true,
		},
		{
			name: "given a valid command with dep should run",
			tasks: []models.Task{
				{
					Name:   "mytask",
					Script: "somecmd",
				},
				{
					Name:      "mytask2",
					Script:    "somecmd2",
					Dir:       ".",
					DependsOn: []string{"mytask"},
				},
			},
			taskName:         "mytask2",
			expectedTasksRun: 2,
		},
		{
			name: "given a valid command with run always set, should only run always",
			tasks: []models.Task{
				{
					Name:              "setup",
					Script:            "somecmd",
					RequiredBehaviour: models.RequiredBehaviourAlways,
				},
				{
					Name:      "mytask",
					Script:    "somecmd",
					DependsOn: []string{"setup"},
				},
				{
					Name:      "mytask2",
					Script:    "somecmd2",
					Dir:       ".",
					DependsOn: []string{"mytask", "setup"},
				},
			},
			taskName:         "mytask2",
			expectedTasksRun: 4,
		},
		{
			name: "given a valid command with run once set, should only run once",
			tasks: []models.Task{
				{
					Name:              "setup",
					Script:            "somecmd",
					RequiredBehaviour: models.RequiredBehaviourOnce,
				},
				{
					Name:      "mytask",
					Script:    "somecmd",
					DependsOn: []string{"setup"},
				},
				{
					Name:      "mytask2",
					Script:    "somecmd2",
					Dir:       ".",
					DependsOn: []string{"mytask", "setup"},
				},
			},
			taskName:         "mytask2",
			expectedTasksRun: 3,
		},
	}
}

func TestRunAsync(t *testing.T) {
	for _, tt := range testCases() {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			for i := range tt.tasks {
				tt.tasks[i].DepsBehaviour = models.DependencyBehaviourAsync
			}
			runner, err := NewRunner(tt.tasks, "")
			if (err != nil) != tt.expectedParseError {
				t.Fatalf("expected error %v, got %v", tt.expectedParseError, err)
			}
			if err != nil {
				return
			}
			scriptRunner := &mockScriptRunner{returns: tt.err}
			runner.scriptRunner = scriptRunner
			err = runner.Run(context.Background(), tt.taskName, nil)
			if (err != nil) != tt.expectedRunError {
				t.Fatalf("expected error %v, got %v", tt.expectedRunError, err)
			}
			if scriptRunner.calls != tt.expectedTasksRun {
				t.Fatalf("expected %d task runs got %d", tt.expectedTasksRun, scriptRunner.calls)
			}
		})
	}
}
func TestRun(t *testing.T) {
	for _, tt := range testCases() {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			runner, err := NewRunner(tt.tasks, "")
			if (err != nil) != tt.expectedParseError {
				t.Fatalf("expected error %v, got %v", tt.expectedParseError, err)
			}
			if err != nil {
				return
			}
			scriptRunner := &mockScriptRunner{returns: tt.err}
			runner.scriptRunner = scriptRunner
			err = runner.Run(context.Background(), tt.taskName, nil)
			if (err != nil) != tt.expectedRunError {
				t.Fatalf("expected error %v, got %v", tt.expectedRunError, err)
			}
			if scriptRunner.calls != tt.expectedTasksRun {
				t.Fatalf("expected %d task runs got %d", tt.expectedTasksRun, scriptRunner.calls)
			}
		})
	}
}

func TestRunWithInputs(t *testing.T) {
	t.Run("given a required input is not provided, return an error", func(t *testing.T) {
		runner, err := NewRunner(models.Tasks{
			{
				Name:   "task",
				Script: "somecmd",
				Inputs: []string{"FOO"},
			},
		}, "")
		if err != nil {
			t.Fatal(err)
		}
		err = runner.Run(context.Background(), "task", nil)
		if err == nil {
			t.Fatal("expected an error got non")
		}
	})
	t.Run("given a required input is provided as an argument, run the task", func(t *testing.T) {
		runner, err := NewRunner(models.Tasks{
			{
				Name:   "task",
				Script: "somecmd",
				Inputs: []string{"FOO"},
			},
		}, "")
		if err != nil {
			t.Fatal(err)
		}
		scriptRunner := &mockScriptRunner{}
		runner.scriptRunner = scriptRunner
		err = runner.Run(context.Background(), "task", []string{"bar"})
		if err != nil {
			t.Fatal(err)
		}
		if scriptRunner.calls != 1 {
			t.Fatal("task was not run")
		}
	})
	t.Run("given a required input is provided as an environment variable, run the task", func(t *testing.T) {
		runner, err := NewRunner(models.Tasks{
			{
				Name:   "task",
				Script: "somecmd",
				Inputs: []string{"FOO"},
			},
		}, "")
		if err != nil {
			t.Fatal(err)
		}
		t.Setenv("FOO", "BAR")
		scriptRunner := &mockScriptRunner{}
		runner.scriptRunner = scriptRunner
		err = runner.Run(context.Background(), "task", nil)
		if err != nil {
			t.Fatal(err)
		}
		if scriptRunner.calls != 1 {
			t.Fatal("task was not run")
		}
	})
}
