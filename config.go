package main

type Config struct {
	Programs map[string]Program `yaml:"programs" required:"true"`
}

type Program struct {
	Exec []string `yaml:"exec" required:"true"`
}

type Layout struct {
	Workspaces map[string]Workspace `yaml:"workspaces" required:"true"`
}

type Workspace []interface{}

type Operation struct {
	Program string
	Args    []string
}
