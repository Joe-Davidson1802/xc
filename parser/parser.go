package parser

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/joerdav/xc/models"
)

// ErrNoTasksHeading is returned if the markdown contains no xc block
var ErrNoTasksHeading = errors.New("no xc block found")

const (
	trimValues       = "_*` "
	codeBlockStarter = "```"
)

type parser struct {
	scanner               *bufio.Scanner
	tasks                 models.Tasks
	currTask              models.Task
	rootHeadingLevel      int
	nextLine, currentLine string
	reachedEnd            bool
}

func (p *parser) Parse() (tasks models.Tasks, err error) {
	ok := true
	for ok {
		ok, err = p.parseTask()
		if err != nil || !ok {
			break
		}
	}
	tasks = p.tasks
	return
}

func (p *parser) scan() bool {
	p.currentLine = p.nextLine
	if p.reachedEnd {
		return false
	}
	if !p.scanner.Scan() {
		p.reachedEnd = true
		p.nextLine = ""
		return true
	}
	p.nextLine = p.scanner.Text()
	return true
}

func stringOnlyContains(input string, matcher rune) bool {
	if len(input) == 0 {
		return false
	}
	for i := range input {
		if []rune(input)[i] != matcher {
			return false
		}
	}
	return true
}

func (p *parser) parseAltHeading(advance bool) (ok bool, level int, text string) {
	t := strings.TrimSpace(p.currentLine)
	n := strings.TrimSpace(p.nextLine)
	if stringOnlyContains(n, '-') {
		ok = true
		level = 2
		text = t
	}
	if stringOnlyContains(n, '=') {
		ok = true
		level = 1
		text = t
	}
	if !advance || !ok {
		return
	}
	p.scan()
	p.scan()
	return
}

func (p *parser) parseHeading(advance bool) (ok bool, level int, text string) {
	ok, level, text = p.parseAltHeading(advance)
	if ok {
		return
	}
	t := strings.TrimSpace(p.currentLine)
	s := strings.Fields(t)
	if len(s) < 2 || len(s[0]) < 1 || strings.Count(s[0], "#") != len(s[0]) {
		return
	}
	ok = true
	level = len(s[0])
	text = strings.Join(s[1:], " ")
	if !advance {
		return
	}
	p.scan()
	return
}

// AttributeType represents metadata related to a Task.
//
//	# Tasks
//	## Task1
//	AttributeName: AttributeValue
//	```
//	script
//	```
type AttributeType int

const (
	// AttributeTypeEnv sets the environment variables for a Task.
	// It can be represented by an attribute with name `environment` or `env`.
	AttributeTypeEnv AttributeType = iota
	// AttributeTypeDir sets the working directory for a Task.
	// It can be represented by an attribute with name `directory` or `dir`.
	AttributeTypeDir
	// AttributeTypeReq sets the required Tasks for a Task, they will run
	// prior to the execution of the selected task.
	// It can be represented by an attribute with name `requires` or `req`.
	AttributeTypeReq
	// AttributeTypeInp sets the required inputs for a Task, inputs can be provided
	// as commandline arguments or environment variables.
	AttributeTypeInp
	// AttributeTypeRun sets the tasks requiredBehaviour, can be always or once.
	// Default is always
	AttributeTypeRun
	// AttributeTypeRunDeps sets the tasks dependenciesBehaviour, can be sync or async.
	AttributeTypeRunDeps
	// AttributeTypeInteractive indicates if this is an interactive task
	// if it is, then logs are not prefixed and the stdout/stderr are passed directly
	// from the OS
	AttributeTypeInteractive
)

var attMap = map[string]AttributeType{
	"req":             AttributeTypeReq,
	"requires":        AttributeTypeReq,
	"env":             AttributeTypeEnv,
	"environment":     AttributeTypeEnv,
	"dir":             AttributeTypeDir,
	"directory":       AttributeTypeDir,
	"inputs":          AttributeTypeInp,
	"run":             AttributeTypeRun,
	"rundeps":         AttributeTypeRunDeps,
	"rundependencies": AttributeTypeRunDeps,
	"interactive":     AttributeTypeInteractive,
}

func (p *parser) parseAttribute() (bool, error) {
	a, rest, found := strings.Cut(p.currentLine, ":")
	if !found {
		return false, nil
	}
	ty, ok := attMap[strings.ToLower(strings.Trim(a, trimValues))]
	if !ok {
		return false, nil
	}
	switch ty {
	case AttributeTypeInp:
		vs := strings.Split(rest, ",")
		for _, v := range vs {
			p.currTask.Inputs = append(p.currTask.Inputs, strings.Trim(v, trimValues))
		}
	case AttributeTypeReq:
		vs := strings.Split(rest, ",")
		for _, v := range vs {
			p.currTask.DependsOn = append(p.currTask.DependsOn, strings.Trim(v, trimValues))
		}
	case AttributeTypeEnv:
		vs := strings.Split(rest, ",")
		for _, v := range vs {
			p.currTask.Env = append(p.currTask.Env, strings.Trim(v, trimValues))
		}
	case AttributeTypeDir:
		if p.currTask.Dir != "" {
			return false, fmt.Errorf("directory appears more than once for %s", p.currTask.Name)
		}
		s := strings.Trim(rest, trimValues)
		p.currTask.Dir = s
	case AttributeTypeRun:
		s := strings.Trim(rest, trimValues)
		r, ok := models.ParseRequiredBehaviour(s)
		if !ok {
			return false, fmt.Errorf("run contains invalid behaviour %q should be (always, once): %s", s, p.currTask.Name)
		}
		p.currTask.RequiredBehaviour = r
	case AttributeTypeRunDeps:
		s := strings.Trim(rest, trimValues)
		r, ok := models.ParseDepsBehaviour(s)
		if !ok {
			return false, fmt.Errorf("runDeps contains invalid behaviour %q should be (sync, async): %s", s, p.currTask.Name)
		}
		p.currTask.DepsBehaviour = r
	case AttributeTypeInteractive:
		s := strings.Trim(rest, trimValues)
		p.currTask.Interactive = s == "true"
	}
	p.scan()
	return true, nil
}

func (p *parser) parseCodeBlock() error {
	t := p.currentLine
	if len(t) < 3 || t[:3] != codeBlockStarter {
		return nil
	}
	if len(p.currTask.Script) > 0 {
		return fmt.Errorf("command block already exists for task %s", p.currTask.Name)
	}
	var ended bool
	for p.scan() {
		if len(p.currentLine) >= 3 && p.currentLine[:3] == codeBlockStarter {
			ended = true
			break
		}
		if strings.TrimSpace(p.currentLine) != "" {
			p.currTask.Script += p.currentLine + "\n"
		}
	}
	if !ended {
		return fmt.Errorf("command block in task %s was not ended", p.currTask.Name)
	}
	p.scan()
	return nil
}

func (p *parser) findTaskHeading() (heading string, done bool, err error) {
	for {
		tok, level, text := p.parseHeading(true)
		if !tok || level > p.rootHeadingLevel+1 {
			if !p.scan() {
				return "", false, fmt.Errorf("failed to read file: %w", p.scanner.Err())
			}
			continue
		}
		if level <= p.rootHeadingLevel {
			return "", true, nil
		}
		return strings.Trim(text, trimValues), false, nil
	}
}

func (p *parser) parseTaskBody() (bool, error) {
	for {
		ok, err := p.parseAttribute()
		if err != nil {
			return false, err
		}
		if p.reachedEnd {
			// parse attribute again in case it is on the last line
			_, err = p.parseAttribute()
			return false, err
		}
		if ok {
			continue
		}
		err = p.parseCodeBlock()
		if err != nil {
			return false, err
		}
		tok, level, _ := p.parseHeading(false)
		if tok && level <= p.rootHeadingLevel {
			return false, nil
		}
		if tok && level == p.rootHeadingLevel+1 {
			return true, nil
		}
		if strings.TrimSpace(p.currentLine) != "" {
			p.currTask.Description = append(p.currTask.Description, strings.Trim(p.currentLine, trimValues))
		}
		if !p.scan() {
			return false, nil
		}
	}
}

func (p *parser) parseTask() (ok bool, err error) {
	p.currTask = models.Task{}
	heading, done, err := p.findTaskHeading()
	if err != nil || done {
		return
	}
	p.currTask.Name = heading
	ok, err = p.parseTaskBody()
	if err != nil {
		return
	}
	if len(p.currTask.Script) < 1 && len(p.currTask.DependsOn) < 1 {
		err = fmt.Errorf("task %s has no commands or required tasks", p.currTask.Name)
		return
	}
	p.tasks = append(p.tasks, p.currTask)
	return
}

// NewParser will read from r until it finds a valid xc heading block.
// If no block is found an error is returned.
func NewParser(r io.Reader, heading string) (p parser, err error) {
	p.scanner = bufio.NewScanner(r)
	for p.scan() {
		ok, level, text := p.parseHeading(true)
		if !ok || !strings.EqualFold(strings.TrimSpace(text), strings.TrimSpace(heading)) {
			continue
		}
		p.rootHeadingLevel = level
		return
	}
	err = ErrNoTasksHeading
	return
}
