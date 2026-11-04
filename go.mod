module github.com/baka-world/dsh-gtk

go 1.27.3

require github.com/egoist/mygo v0.7.5-0.20261104132624-738acd7f7d2a

require (
	github.com/ebitengine/purego v0.11.1 // indirect
	golang.org/x/net v0.59.0 // indirect
	golang.org/x/text v0.43.0 // indirect
)

tool github.com/egoist/mygo/cmd/mygo

replace github.com/egoist/mygo => github.com/baka-world/mygo v0.7.5-0.20261104132624-738acd7f7d2a
