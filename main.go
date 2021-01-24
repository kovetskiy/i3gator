package main

import (
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"

	"gopkg.in/yaml.v2"

	"github.com/docopt/docopt-go"
	"github.com/kovetskiy/ko"
	"github.com/reconquest/executil-go"
	"github.com/reconquest/karma-go"
)

var (
	version = "[manual build]"
	usage   = "i3gator " + version + os.ExpandEnv(`

Usage:
  i3gator [options] <layout>
  i3gator -h | --help
  i3gator --version

Options:
  -h --help          Show this screen.
  --version          Show version.
  -c --config <dir>  Path to configs directory. [default: $HOME/.config/i3gator/]
`)
)

func main() {
	args, err := docopt.Parse(usage, nil, true, version, false)
	if err != nil {
		panic(err)
	}

	var (
		configsDir = args["--config"].(string)
		layoutName = args["<layout>"].(string)
	)

	var config Config
	err = ko.Load(
		filepath.Join(configsDir, "i3gator.conf"),
		&config,
		yaml.Unmarshal,
	)
	if err != nil {
		log.Fatalln(err)
	}

	for key, value := range DefaultPrograms {
		if _, ok := config.Programs[key]; !ok {
			config.Programs[key] = value
		}
	}

	var layout Layout
	err = ko.Load(
		filepath.Join(configsDir, "layouts", layoutName+".conf"),
		&layout,
		yaml.Unmarshal,
	)
	if err != nil {
		log.Fatalln(err)
	}

	workspaces := map[string]protocol{}
	for label, workspace := range layout.Workspaces {
		check := decodeOperations(config, label, workspace.Check)
		do := decodeOperations(config, label, workspace.Do)

		workspaces[label] = protocol{
			check: check,
			do:    do,
		}
	}

	for label, protocol := range workspaces {
		createWorkspace(config.Programs, label, protocol)
	}
}

type protocol struct {
	check []Operation
	do    []Operation
}

func createWorkspace(programs map[string]Program, label string, protocol protocol) {
	if label != "-" {
		switchWorkspace(label)
	}
	switchWorkspace(label)

	cwd, err := os.Getwd()
	if err != nil {
		log.Fatalln(err)
	}

	// should DO if there are no checks
	shouldDo := len(protocol.check) == 0

	buffer := map[string]string{}
	for _, op := range protocol.check {
		if op.Program == "cwd" {
			cwd = expandCWD(op.Args[0])
			continue
		}

		code := operate(label, programs, buffer, op, cwd, "check")
		if code != 0 {
			// if a check failed, we definitely should DO
			shouldDo = true
			break
		}
	}

	if !shouldDo {
		return
	}

	vars := map[string]string{}
	for _, op := range protocol.do {
		if op.Program == "cwd" {
			cwd = expandCWD(op.Args[0])
			continue
		}

		code := operate(label, programs, vars, op, cwd, "do")
		if code != 0 {
			log.Fatalf("%q failed with exit code: %d", op, code)
		}
	}
}

func expandCWD(dir string) string {
	dir = strings.Replace(
		dir,
		"~/",
		strings.TrimRight(os.Getenv("HOME"), "/")+"/",
		1,
	)

	abs, err := filepath.Abs(dir)
	if err != nil {
		log.Fatalln(karma.Format(
			err,
			"unable to get abs path for %s", dir,
		))
	}

	return abs
}

func switchWorkspace(label string) {
	_, _, err := executil.Run(exec.Command("i3-msg", "workspace "+label))
	if err != nil {
		log.Fatalln(err)
	}
}

func operate(
	label string,
	programs map[string]Program,
	vars map[string]string,
	op Operation,
	cwd string,
	act string,
) int {
	args := getArgs(programs, vars, op)

	log.Printf("[%s %s] %s: %v", label, act, op.Program, args)

	if len(args) == 0 {
		panic("size of args is zero (expected 1+) for program " + op.Program)
	}

	path, err := exec.LookPath(args[0])
	if err != nil {
		log.Fatalln(karma.Format(
			err,
			"unable to find path for %s", args[0],
		))
	}

	// we can't use exec.Command here becauset it waits for all child FDs

	reader, writer, err := os.Pipe()
	if err != nil {
		log.Fatalln(karma.Format(
			err,
			"unable to pipe",
		))
	}

	stdout := []byte{}
	worker := &sync.WaitGroup{}
	worker.Add(1)
	go func() {
		defer worker.Done()

		data, err := ioutil.ReadAll(reader)
		if err != nil {
			log.Fatalln(karma.Format(
				err,
				"unable to read out pipe",
			))
		}

		stdout = data
	}()

	process, err := os.StartProcess(path, args, &os.ProcAttr{
		Dir: cwd,
		Env: os.Environ(),
		Files: []*os.File{
			nil,
			writer,
			os.Stderr,
		},
	})
	if err != nil {
		log.Fatalln(err)
	}

	state, err := process.Wait()
	if err != nil {
		log.Fatalln(err)
	}

	writer.Close()

	worker.Wait()

	result := strings.TrimSpace(string(stdout))
	varName := programs[op.Program].Assign
	if varName == "" {
		varName = op.Program
	}

	vars[varName] = result

	log.Printf(
		"[%s %s] %s: %v exit_code=%d stdout=%s",
		label,
		act,
		op.Program,
		args,
		state.ExitCode(),
		result,
	)

	return state.ExitCode()
}

func getArgs(programs map[string]Program, vars map[string]string, op Operation) []string {
	program := programs[op.Program]
	args := []string{}
	for _, cmd := range program.Exec {
		if strings.HasPrefix(cmd, "<") && strings.HasSuffix(cmd, ">") {
			name := strings.Trim(cmd, "<>")
			if name == "args" {
				for _, arg := range op.Args {
					args = append(args, arg)
				}
			} else {
				value, ok := vars[name]
				if !ok {
					log.Fatalf("program %s uses variable <%s> that is not defined", op.Program, name)
				}

				args = append(args, value)
			}
		} else {
			args = append(args, cmd)
		}
	}

	return args
}

func decodeOperations(
	config Config,
	label string,
	input []interface{},
) []Operation {
	ops := []Operation{}
	for _, raw := range input {
		var op Operation
		switch typed := raw.(type) {
		case string:
			op = Operation{
				Program: typed,
			}

		case map[interface{}]interface{}:
			var program string
			var args []string
			for key, value := range typed {
				if key, ok := key.(string); ok {
					program = key
				} else {
					log.Fatalf("unexpected type of key %#v: %T", key, key)
				}

				switch typedValue := value.(type) {
				case string:
					args = []string{typedValue}
				case []string:
					args = typedValue
				case []interface{}:
					for _, value := range typedValue {
						args = append(args, fmt.Sprint(value))
					}
				default:
					log.Fatalf(
						"unexpected type of value for key %s: %T %#v",
						key,
						typedValue,
						typedValue,
					)
				}
			}

			op = Operation{
				Program: program,
				Args:    args,
			}

		default:
			log.Fatalf("unexpected type of %#v: %T", raw, typed)
		}

		if _, ok := config.Programs[op.Program]; !ok {
			if op.Program != "cwd" {
				log.Fatalf("unknown program specified: %s", op.Program)
			}
		}

		ops = append(ops, op)
	}

	return ops
}
