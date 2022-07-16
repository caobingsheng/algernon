package engine

import (
	"bytes"
	"context"
	"encoding/json"
	"net/rpc"
	"net/rpc/jsonrpc"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"

	"github.com/creachadair/jrpc2"
	"github.com/creachadair/jrpc2/channel"
	"github.com/natefinch/pie"
	lua "github.com/xyproto/gopher-lua"
	"github.com/xyproto/textoutput"
)

type luaPlugin struct {
	client *rpc.Client
}

const namespace = "Lua"

func (lp *luaPlugin) LuaCode(pluginPath string) (luacode string, err error) {
	return luacode, lp.client.Call(namespace+".Code", pluginPath, &luacode)
}

func (lp *luaPlugin) LuaHelp() (luahelp string, err error) {
	return luahelp, lp.client.Call(namespace+".Help", "", &luahelp)
}

// LoadPluginFunctions takes a Lua state and a TextOutput
// (the TextOutput struct should be nil if not in a REPL)
func (ac *Config) LoadPluginFunctions(L *lua.LState, o *textoutput.TextOutput) {

	// Expose the functionality of a given plugin (executable file).
	// If on Windows, ".exe" is added to the path.
	// Returns true of successful.
	L.SetGlobal("Plugin", L.NewFunction(func(L *lua.LState) int {
		path := L.ToString(1)
		givenPath := path
		if runtime.GOOS == "windows" {
			path = path + ".exe"
		}
		if !ac.fs.Exists(path) {
			path = filepath.Join(ac.serverDirOrFilename, path)
		}

		// Keep the plugin running in the background?
		keepRunning := false
		if L.GetTop() >= 2 {
			keepRunning = L.ToBool(2)
		}

		// Connect with the Plugin
		client, err := pie.StartProviderCodec(jsonrpc.NewClientCodec, os.Stderr, path)
		if err != nil {
			if o != nil {
				o.Err("[Plugin] Could not run plugin!")
				o.Err("Error: " + err.Error())
			}
			L.Push(lua.LBool(false)) // Fail
			return 1                 // number of results
		}

		if !keepRunning {
			// Close the client once this function has completed
			defer client.Close()
		}

		p := &luaPlugin{client}

		// Retrieve the Lua code
		luacode, err := p.LuaCode(givenPath)
		if err != nil {
			if o != nil {
				o.Err("[Plugin] Could not call the LuaCode function!")
				o.Err("Error: " + err.Error())
			}
			L.Push(lua.LBool(false)) // Fail
			return 1                 // number of results
		}

		// Retrieve the help text
		luahelp, err := p.LuaHelp()
		if err != nil {
			if o != nil {
				o.Err("[Plugin] Could not call the LuaHelp function!")
				o.Err("Error: " + err.Error())
			}
			L.Push(lua.LBool(false)) // Fail
			return 1                 // number of results
		}

		// Run luacode on the current LuaState
		luacode = strings.TrimSpace(luacode)
		if L.DoString(luacode) != nil {
			if o != nil {
				o.Err("[Plugin] Error in Lua code provided by plugin!")
				o.Err("Error: " + err.Error())
			}
			L.Push(lua.LBool(false)) // Fail
			return 1                 // number of results
		}

		// If in a REPL, output the Plugin help text
		if o != nil {
			luahelp = strings.TrimSpace(luahelp)
			// Add syntax highlighting and output the text
			o.Println(highlight(o, luahelp))
		}

		L.Push(lua.LBool(true)) // Success
		return 1                // number of results
	}))

	// Retrieve the code from the Lua.Code function of the plugin
	L.SetGlobal("PluginCode", L.NewFunction(func(L *lua.LState) int {
		path := L.ToString(1)
		givenPath := path
		if runtime.GOOS == "windows" {
			path = path + ".exe"
		}
		if !ac.fs.Exists(path) {
			path = filepath.Join(ac.serverDirOrFilename, path)
		}

		// Keep the plugin running in the background?
		keepRunning := false
		if L.GetTop() >= 2 {
			keepRunning = L.ToBool(2)
		}

		// Connect with the Plugin
		client, err := pie.StartProviderCodec(jsonrpc.NewClientCodec, os.Stderr, path)
		if err != nil {
			if o != nil {
				o.Err("[PluginCode] Could not run plugin!")
				o.Err("Error: " + err.Error())
			}
			L.Push(lua.LString("")) // Fail
			return 1                // number of results
		}
		if !keepRunning {
			// Close the client once this function has completed
			defer client.Close()
		}

		p := &luaPlugin{client}

		// Retrieve the Lua code
		luacode, err := p.LuaCode(givenPath)
		if err != nil {
			if o != nil {
				o.Err("[PluginCode] Could not call the LuaCode function!")
				o.Err("Error: " + err.Error())
			}
			L.Push(lua.LString("")) // Fail
			return 1                // number of results
		}

		L.Push(lua.LString(luacode))
		return 1 // number of results
	}))

	// Call a function exposed by a plugin (executable file)
	// Returns either nil (fail) or a string (success)
	L.SetGlobal("CallPlugin", L.NewFunction(func(L *lua.LState) int {
		if L.GetTop() < 2 {
			if o != nil {
				o.Err("[CallPlugin] Needs at least 2 arguments")
			}
			L.Push(lua.LString("")) // Fail
			return 1                // number of results
		}

		path := L.ToString(1)
		if runtime.GOOS == "windows" {
			path = path + ".exe"
		}
		if !ac.fs.Exists(path) {
			path = filepath.Join(ac.serverDirOrFilename, path)
		}

		fn := L.ToString(2)

		var args []lua.LValue
		if L.GetTop() > 2 {
			for i := 3; i <= L.GetTop(); i++ {
				args = append(args, L.Get(i))
			}
		}

		// Connect with the Plugin
		logto := os.Stderr
		if o != nil {
			logto = os.Stdout
		}

		// Keep the plugin running in the background?
		keepRunning := false

		client, err := pie.StartProviderCodec(jsonrpc.NewClientCodec, logto, path)
		if err != nil {
			if o != nil {
				o.Err("[CallPlugin] Could not run plugin!")
				o.Err("Error: " + err.Error())
			}
			L.Push(lua.LString("")) // Fail
			return 1                // number of results
		}

		if !keepRunning {
			// Close the client once this function has completed
			defer client.Close()
		}

		jsonargs, err := json.Marshal(args)
		if err != nil {
			if o != nil {
				o.Err("[CallPlugin] Error when marshalling arguments to JSON")
				o.Err("Error: " + err.Error())
			}
			L.Push(lua.LString("")) // Fail
			return 1                // number of results
		}

		// Attempt to call the given function name
		var jsonreply []byte
		if err := client.Call(namespace+"."+fn, jsonargs, &jsonreply); err != nil {
			if o != nil {
				o.Err("[CallPlugin] Error when calling function!")
				o.Err("Function: " + namespace + "." + fn)
				o.Err("JSON Arguments: " + string(jsonargs))
				o.Err("Error: " + err.Error())
			}
			L.Push(lua.LString("")) // Fail
			return 1                // number of results
		}

		L.Push(lua.LString(jsonreply)) // Resulting string
		return 1                       // number of results
	}))

	// let cmd use arguments to startup
	L.SetGlobal("RPC", L.NewFunction(func(L *lua.LState) int {
		if L.GetTop() < 3 {
			if o != nil {
				o.Err("[CallPlugin] Needs at least 3 arguments")
			}
			L.Push(lua.LString("")) // Fail
			return 1                // number of results
		}

		path := L.ToString(1)
		if runtime.GOOS == "windows" {
			path = path + ".exe"
		}
		if !ac.fs.Exists(path) {
			path = filepath.Join(ac.serverDirOrFilename, path)
		}

		fn := L.ToString(2)

		argStr := ""
		cmdArgsTable := L.ToTable(3)

		cmdArgs := make([]string, cmdArgsTable.Len())
		cmdArgsTable.ForEach(func(_, value lua.LValue) {
			arg := value.String()
			cmdArgs = append(cmdArgs, arg)
			argStr += arg
		})

		var args []lua.LValue
		if L.GetTop() > 3 {
			for i := 4; i <= L.GetTop(); i++ {
				args = append(args, L.Get(i))
			}
		}

		keepRunning := false

		// TODO 启动插件
		cmd := exec.Command(path, cmdArgs...)
		in, err := cmd.StdinPipe()
		if err != nil {
			if o != nil {
				o.Err("[CallPlugin] Could not run plugin!")
				o.Err("Error: " + err.Error())
			}
			L.Push(lua.LString("")) // Fail
			return 1
		}

		out, err := cmd.StdoutPipe()
		if err != nil {
			if o != nil {
				o.Err("[CallPlugin] Could not run plugin!")
				o.Err("Error: " + err.Error())
			}
			L.Push(lua.LString("")) // Fail
			return 1
		}
		err = cmd.Start()
		if err != nil {
			if o != nil {
				o.Err("[CallPlugin] Could not run plugin!")
				o.Err("Error: " + err.Error())
			}
			L.Push(lua.LString("")) // Fail
			return 1
		}
		client := jrpc2.NewClient(channel.Header("")(out, in), &jrpc2.ClientOptions{})

		if client == nil {
			if o != nil {
				o.Err("[RPC] JSONRPC client创建失败")
			}
			L.Push(lua.LString("")) // Fail
			return 1                // number of results
		}
		if !keepRunning {
			defer client.Close()
		}
		jsonargs, err := json.Marshal(args)
		if err != nil {
			if o != nil {
				o.Err("[CallPlugin] Error when marshalling arguments to JSON")
				o.Err("Error: " + err.Error())
			}
			// 	L.Push(lua.LString("")) // Fail
			// 	return 1                // number of results
		}

		params := make([]interface{}, 0)
		buffer := new(bytes.Buffer)
		for i, arg := range args {
			switch arg.Type() {
			case lua.LTNil:
				params = append(params, nil)
				buffer.WriteString("添加 null:" + strconv.Itoa(i) + " _|_ ")

			case lua.LTBool:
				params = append(params, lua.LVAsBool(arg))
				buffer.WriteString("添加 bool:" + strconv.Itoa(i) + " _|_ ")

			case lua.LTNumber:
				params = append(params, lua.LVAsNumber(arg))
				buffer.WriteString("添加 number:" + strconv.Itoa(i) + ":" + arg.String() + " _|_ ")

			case lua.LTString:
				params = append(params, lua.LVAsString(arg))
				buffer.WriteString("添加 string:" + strconv.Itoa(i) + ":" + arg.String() + " _|_ ")

			case lua.LTUserData:
				ut := arg.(*lua.LUserData)
				params = append(params, ut.Value)
				buffer.WriteString("添加 userdata:" + strconv.Itoa(i) + ":" + arg.String() + " _|_ ")

			case lua.LTTable:
				table := arg.(*lua.LTable)
				var tableData interface{}
				table.ForEach(func(keyOrIndex, value lua.LValue) {
					kiType := keyOrIndex.Type()
					buffer.WriteString("添加 table each:" + strconv.Itoa(i) + ":" + keyOrIndex.String() + ":" + kiType.String() + " _|_ ")
					if tableData == nil {
						switch kiType {
						case lua.LTString:
							tableData = make(map[string]interface{}, 0)
						case lua.LTNumber:
							tableData = make([]interface{}, 0)
						default:
							tableData = make([]interface{}, 0)
						}
					}
					switch kiType {
					case lua.LTString:
						tableData.(map[string]interface{})[keyOrIndex.String()] = value
					case lua.LTNumber:
						tableData = append(tableData.([]interface{}), value)
					default:
						tableData = append(tableData.([]interface{}), value)
					}
				})
				table.Metatable.Type()
				if table.Len() <= 0 {
					tableData = make([]interface{}, 0)
				}
				buffer.WriteString("添加 table:" + strconv.Itoa(i) + ":" + arg.String() + " _|_ ")
				params = append(params, tableData)
			}
		}

		// if cmdArgsTable == nil {
		// 	L.Push(lua.LString("是空的"))
		// } else {
		// 	/*
		// 		添加 string:0:http://www.webxml.com.cn/WebServices/TrainTimeWebService.asmx?wsdl
		// 		添加 string:1:getVersionTime
		// 		添加 table:2:table: 0xc000250310
		// 		添加 table:3:table: 0xc000250380
		// 		[null,null,null,null,"http://www.webxml.com.cn/WebServices/TrainTimeWebService.asmx?wsdl","getVersionTime",null,null]
		// 		["http://www.webxml.com.cn/WebServices/TrainTimeWebService.asmx?wsdl","getVersionTime",{"Metatable":{}},{"Metatable":{}}]
		// 	*/
		// 	j, _ := json.Marshal(params)
		// 	buffer.Write(j)
		// 	buffer.WriteString(" _|_ ")
		// 	buffer.Write(jsonargs)
		// 	L.Push(lua.LString(buffer.Bytes()))
		// }
		// return 1
		ctx := context.Background()
		_, err = client.Call(ctx, fn, params)
		if err != nil {
			o.Err("[CallPlugin] Error when calling function!")
			o.Err("Function: " + fn)
			o.Err("JSON Arguments: " + string(jsonargs))
			o.Err("Error: " + err.Error())
		}
		// L.Push(lua.LString(resp.ResultString())) // Resulting string
		L.Push(lua.LString("resp.ResultString()")) // Resulting string
		return 1                                   // number of results
	}))

}
