package run

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/google/shlex"
	"github.com/joerdav/xc/models"
)

const maxDeps = 50

type ScriptRunner interface {
	Execute(ctx context.Context, text string, env, args []string, dir, logPrefix string) error
}

// Runner is responsible for running Tasks.
type Runner struct {
	scriptRunner ScriptRunner
	tasks        models.Tasks
	dir          string
	alreadyRan   map[string]bool
	alreadyRanMu sync.Mutex
}

// NewRunner takes Tasks and returns a Runner.
// If the OS is windows commands will be run using `cmd \C`
// and separated by `&&`.
// Otherwise, commands will be run using `bash -c`
// and separated by `;`.
//
// NewRunner will return an error in the case that Dependent tasks are cyclical,
// invalid or at a larger depth than 50.
func NewRunner(ts models.Tasks, dir string) (runner Runner, err error) {
	runner = Runner{
		scriptRunner: newInterpreter(),
		tasks:        ts,
		dir:          dir,
		alreadyRan:   map[string]bool{},
	}
	for _, t := range ts {
		err = runner.ValidateDependencies(t.Name, []string{})
		if err != nil {
			return
		}
	}
	return
}

const scriptHeader = ` #!/bin/bash
      set -e
      set -o xtrace
`

func taskUsage(task models.Task) string {
	argUsage := fmt.Sprintf("xc %s", task.Name)
	for _, n := range task.Inputs {
		argUsage += fmt.Sprintf(" <%s>", strings.ToLower(n))
	}
	envUsage := ""
	for _, n := range task.Inputs {
		envUsage += fmt.Sprintf("%s=<%s> ", n, strings.ToLower(n))
	}
	envUsage += fmt.Sprintf("xc %s", task.Name)
	return fmt.Sprintf("Task has required inputs:\n\t%s\n\t%s", argUsage, envUsage)
}

func environmentContainsInput(env []string, input string) bool {
	for _, en := range env {
		if strings.Split(en, "=")[0] == input {
			return true
		}
	}
	return false
}

func getInputs(task models.Task, inputs, env []string) ([]string, error) {
	result := []string{}
	for i, n := range task.Inputs {
		// Do the command args contain the input?
		if len(inputs) > i {
			result = append(result, fmt.Sprintf("%v=%v", n, inputs[i]))
			continue
		}
		// Does the task environment contain the input?
		if environmentContainsInput(env, n) {
			continue
		}
		return nil, errors.New(taskUsage(task))
	}
	return result, nil
}

// Run runs a task given a string name.
// Task dependencies will be run first, an error will return if any fail.
// Task commands are run next, in case of a non zero result an error will return.
func (r *Runner) Run(ctx context.Context, name string, inputs []string) error {
	padding, err := r.getLogPadding(name)
	if err != nil {
		return err
	}
	return r.runWithPadding(ctx, name, inputs, padding)
}

func (r *Runner) runWithPadding(ctx context.Context, name string, inputs []string, padding int) error {
	task, ok := r.tasks.Get(name)
	if !ok {
		return fmt.Errorf("task %s not found", name)
	}
	r.alreadyRanMu.Lock()
	if task.RequiredBehaviour == models.RequiredBehaviourOnce && r.alreadyRan[task.Name] {
		r.alreadyRanMu.Unlock()
		fmt.Printf("task %q ran already: skipping\n", task.Name)
		return nil
	}
	r.alreadyRan[task.Name] = true
	r.alreadyRanMu.Unlock()
	env := os.Environ()
	env = append(env, task.Env...)
	inp, err := getInputs(task, inputs, env)
	if err != nil {
		return err
	}
	runFunc := r.runDepsSync
	if task.DepsBehaviour == models.DependencyBehaviourAsync {
		runFunc = r.runDepsAsync
	}
	if err := runFunc(ctx, padding, task.DependsOn...); err != nil {
		return err
	}
	if len(task.Script) == 0 {
		return nil
	}
	env = append(env, inp...)

	var prefix string
	if !task.Interactive {
		prefix = fmt.Sprintf("%*s", padding, strings.TrimSpace(task.Name))
	}
	return r.scriptRunner.Execute(ctx, task.Script, env, inputs, r.getExecutionPath(task), prefix)
}

func (r *Runner) runDepsSync(ctx context.Context, padding int, dependencies ...string) error {
	for _, t := range dependencies {
		ta, err := shlex.Split(t)
		if err != nil {
			return err
		}
		if err := r.runWithPadding(ctx, ta[0], ta[1:], padding); err != nil {
			return err
		}
	}
	return nil
}

func (r *Runner) runDepsAsync(ctx context.Context, padding int, dependencies ...string) error {
	var wg sync.WaitGroup
	errs := make([]error, len(dependencies))
	for i, t := range dependencies {
		wg.Add(1)
		go func(index int, task string) {
			defer wg.Done()
			ta, err := shlex.Split(task)
			if err != nil {
				errs[index] = err
				return
			}

			errs[index] = r.runWithPadding(ctx, ta[0], ta[1:], padding)
		}(i, t)
	}

	wg.Wait()
	return errors.Join(errs...)
}

func (r *Runner) getLogPadding(name string) (int, error) {
	task, ok := r.tasks.Get(name)
	if !ok {
		return 0, fmt.Errorf("task %s not found", name)
	}

	maxLen := len(task.Name)
	for _, depName := range task.DependsOn {
		depLen, err := r.getLogPadding(depName)
		if err != nil {
			return maxLen, err
		}
		if depLen > maxLen {
			maxLen = depLen
		}
	}
	return maxLen, nil
}

func (r *Runner) getExecutionPath(task models.Task) string {
	if task.Dir == "" {
		return r.dir
	}
	if filepath.IsAbs(task.Dir) {
		return task.Dir
	}
	return filepath.Join(r.dir, task.Dir)
}

// ValidateDependencies checks that task dependencies follow these rules:
// - No deeper dependency trees than maxDeps.
// - Dependencies must exist as tasks.
// - No cyclical dependencies.
func (r *Runner) ValidateDependencies(task string, prevTasks []string) error {
	if len(prevTasks) >= maxDeps {
		return fmt.Errorf("max dependency depth of %d reached", maxDeps)
	}
	// Check exists
	t, ok := r.tasks.Get(task)
	if !ok {
		return fmt.Errorf("task %s not found", task)
	}
	if t.ParsingError != "" {
		return fmt.Errorf("task %s has a parsing error: %s", task, t.ParsingError)
	}
	for _, t := range t.DependsOn {
		t, _, _ := strings.Cut(t, " ")
		st, ok := r.tasks.Get(t)
		if !ok {
			return fmt.Errorf("task %s not found", t)
		}
		for _, pt := range prevTasks {
			if pt == st.Name {
				return fmt.Errorf("task %s contains a circular dependency", t)
			}
		}
		err := r.ValidateDependencies(st.Name, append([]string{st.Name}, prevTasks...))
		if err != nil {
			return err
		}
	}
	return nil
}
