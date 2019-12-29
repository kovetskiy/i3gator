package main

var DefaultPrograms = map[string]Program{
	"i3": Program{Exec: []string{"i3-msg", "<args>"}},
}
