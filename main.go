package main

import (
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

	workspaces := map[string][]Operation{}
	for label, workspace := range layout.Workspaces {
		ops := decodeWorkspace(config, label, workspace)
		workspaces[label] = ops
	}

	for label, ops := range workspaces {
		createWorkspace(config.Programs, label, ops)
	}
}

func createWorkspace(programs map[string]Program, label string, ops []Operation) {
	switchWorkspace(label)

	cwd, err := os.Getwd()
	if err != nil {
		log.Fatalln(err)
	}

	vars := map[string]string{}
	for _, op := range ops {
		if op.Program == "cwd" {
			cwd = expandCWD(op.Args[0])
			continue
		}

		operate(label, programs, vars, op, cwd)
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

func operate(label string, programs map[string]Program, vars map[string]string, op Operation, cwd string) {
	args := getArgs(programs, vars, op)

	log.Printf("[%s] %s: %v", label, op.Program, args)

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

	if state.ExitCode() != 0 {
		log.Fatalf(
			"%v exited with non-zero exit code %d",
			args,
			state.ExitCode(),
		)
	}

	writer.Close()

	worker.Wait()

	vars[op.Program] = strings.TrimSpace(string(stdout))
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

func decodeWorkspace(
	config Config,
	label string,
	workspace Workspace,
) []Operation {
	ops := []Operation{}
	for _, raw := range workspace {
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
				default:
					log.Fatalf("unexpected type of value for key: %s", key)
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
