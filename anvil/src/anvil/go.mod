module github.com/jeffwilliams/anvil

go 1.22.3

//replace github.com/leaanthony/go-ansi-parser => /tmp/go-ansi-parser-jeff

//replace github.com/sarpdag/boyermoore => /home/jefwill3/src/boyermoore
replace github.com/sarpdag/boyermoore => github.com/jeffwilliams/boyermoore v0.0.0-20220817021623-63ad6ff520f8

//replace github.com/jeffwilliams/syn => /home/jefwill3/src/syn

require (
	gioui.org v0.0.0-20230502183330-59695984e53c
	github.com/alecthomas/chroma v0.10.0
	github.com/alecthomas/chroma/v2 v2.14.0
	github.com/armon/go-radix v0.0.0-20180808171621-7fddfc383310
	github.com/ddkwork/golibrary v0.0.49
	github.com/flopp/go-findfont v0.1.0
	github.com/go-text/typesetting v0.0.0-20230413204129-b4f0492bf7ae
	github.com/jeffwilliams/syn v0.1.6
	github.com/jszwec/csvutil v1.6.0
	github.com/leaanthony/go-ansi-parser v1.6.1
	github.com/ogier/pflag v0.0.1
	github.com/pelletier/go-toml v1.9.5
	github.com/pkg/profile v1.6.0
	github.com/sarpdag/boyermoore v0.0.0-20210425165139-a89ed1b5913b
	github.com/stretchr/testify v1.9.0
	golang.org/x/crypto v0.23.0
	golang.org/x/image v0.16.0
)

require (
	gioui.org/cpu v0.0.0-20210817075930-8d6a761490d2 // indirect
	gioui.org/shader v1.0.6 // indirect
	github.com/axgle/mahonia v0.0.0-20180208002826-3358181d7394 // indirect
	github.com/davecgh/go-spew v1.1.2-0.20180830191138-d8f796af33cc // indirect
	github.com/dc0d/caseconv v0.5.0 // indirect
	github.com/dlclark/regexp2 v1.11.0 // indirect
	github.com/dop251/goja v0.0.0-20240516125602-ccbae20bcec2 // indirect
	github.com/fatih/color v1.17.0 // indirect
	github.com/go-sourcemap/sourcemap v2.1.4+incompatible // indirect
	github.com/google/go-cmp v0.6.0 // indirect
	github.com/google/pprof v0.0.0-20240525223248-4bfdf5a9a2af // indirect
	github.com/gorilla/websocket v1.5.1 // indirect
	github.com/hupe1980/golog v0.0.2 // indirect
	github.com/hupe1980/socks v0.0.9 // indirect
	github.com/mattn/go-colorable v0.1.13 // indirect
	github.com/mattn/go-isatty v0.0.20 // indirect
	github.com/mattn/go-sqlite3 v1.14.22 // indirect
	github.com/pmezard/go-difflib v1.0.1-0.20181226105442-5d4384ee4fb2 // indirect
	github.com/rivo/uniseg v0.4.7 // indirect
	golang.org/x/exp v0.0.0-20240525044651-4c93da0ed11d // indirect
	golang.org/x/exp/shiny v0.0.0-20220827204233-334a2380cb91 // indirect
	golang.org/x/mod v0.17.0 // indirect
	golang.org/x/net v0.25.0 // indirect
	golang.org/x/sys v0.20.0 // indirect
	golang.org/x/text v0.15.0 // indirect
	golang.org/x/tools v0.21.0 // indirect
	gopkg.in/yaml.v3 v3.0.1 // indirect
	mvdan.cc/gofumpt v0.6.0 // indirect
)
