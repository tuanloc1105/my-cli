module find-everything

go 1.24.4

require (
	common-module v0.0.0
	github.com/inconshreveable/mousetrap v1.1.0 // indirect
	github.com/spf13/cobra v1.9.1
	github.com/spf13/pflag v1.0.6 // indirect
)

replace common-module => ../common-module
