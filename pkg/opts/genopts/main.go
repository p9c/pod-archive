// This generator reads a podcfg.Configs map and spits out a podcfg.Config struct
package main

import (
	"fmt"
	"github.com/p9c/pod/pkg/opts"
	"github.com/p9c/pod/pkg/spec"
	"go/format"
	"io/ioutil"
	"os"
	"path/filepath"
	"reflect"
	"sort"
)

func main() {
	c := spec.GetConfigs()
	var o string
	var cc opts.ConfigSlice
	for i := range c {
		cc = append(cc, opts.ConfigSliceElement{Opt: c[i], Name: i})
	}
	sort.Sort(cc)
	for i := range cc {
		t := reflect.TypeOf(cc[i].Opt).String()
		// W.Ln(t)
		// split := strings.Split(t, "podcfg.")[1]
		o += fmt.Sprintf("\t%s\t%s\n", cc[i].Name, t)
	}
	var e error
	var out []byte
	var wd string
	generated := fmt.Sprintf(configBase, o)
	if out, e = format.Source([]byte(generated)); e != nil {
		// panic(e)
		fmt.Println(e)
	}
	if wd, e = os.Getwd(); e != nil {
		// panic(e)
	}
	// fmt.Println(string(out), wd)
	if e = ioutil.WriteFile(filepath.Join(wd, "struct.go"), out, 0660); e != nil {
		panic(e)
	}
}

var configBase = `package opts

import (
	"github.com/p9c/opts/binary"
	"github.com/p9c/opts/cmds"
	"github.com/p9c/opts/duration"
	"github.com/p9c/opts/float"
	"github.com/p9c/opts/integer"
	"github.com/p9c/opts/list"
	"github.com/p9c/opts/opt"
	"github.com/p9c/opts/text"
)

// Config defines the configuration items used by pod along with the various components included in the suite
//go:generate go run genopts/main.go
type Config struct {
	// ShowAll is a flag to make the json encoder explicitly define all fields and not just the ones different to the
	// defaults
	ShowAll bool
	// Map is the same data but addressible using its name as found inside the various configuration types, the key is
	// converted to lower case for CLI args
	Map            map[string]opt.Option
	Commands       cmds.Commands
	RunningCommand *cmds.Command
%s}
`
