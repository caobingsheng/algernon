package engine

import (
	"context"
	"encoding/json"
	"fmt"
	"net/rpc"
	"net/rpc/jsonrpc"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/creachadair/jrpc2"
	"github.com/creachadair/jrpc2/channel"
	"github.com/natefinch/pie"
	log "github.com/sirupsen/logrus"
	lua "github.com/xyproto/gopher-lua"
	"github.com/xyproto/textoutput"
)

const (
	NUMBER_OF_RPC_ARGUMENTS = 5 // RPC参数的数量
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
		log.Info("call RPC")

		if L.GetTop() < NUMBER_OF_RPC_ARGUMENTS {
			log.Error(fmt.Sprintf("[RPC] 需要%d个参数. 分别是 程序路径[win不含exe], 程序命令行参数[arg0, arg1], 方法名, 方法参数{name:李四}, 是否需要jsonrpc的Content-Type[true|false]", NUMBER_OF_RPC_ARGUMENTS))
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
		argStr := ""
		cmdArgsTable := L.ToTable(2)

		cmdArgs := make([]string, cmdArgsTable.Len())
		cmdArgsTable.ForEach(func(_, value lua.LValue) {
			arg := value.String()
			cmdArgs = append(cmdArgs, arg)
			argStr += arg
		})

		fn := L.ToString(3)

		argsValue := L.Get(4)
		params := LValue2SerialableValue(&argsValue)

		useJsonRpcContentTypeHeader := L.ToBool(5)

		keepRunning := false

		// 启动插件
		cmd := exec.Command(path, cmdArgs...)
		cmd.Dir = filepath.Dir(path)
		log.Info("启动目录" + cmd.Dir)
		in, err := cmd.StdinPipe()
		if err != nil {
			if o != nil {
				o.Err("[RPC] Could not run plugin!")
				o.Err("Error: " + err.Error())
			}
			L.Push(lua.LString("")) // Fail
			return 1
		}

		out, err := cmd.StdoutPipe()
		if err != nil {
			if o != nil {
				o.Err("[RPC] Could not run plugin!")
				o.Err("Error: " + err.Error())
			}
			L.Push(lua.LString("")) // Fail
			return 1
		}
		err = cmd.Start()
		if err != nil {
			if o != nil {
				o.Err("[RPC] Could not run plugin!")
				o.Err("Error: " + err.Error())
			}
			L.Push(lua.LString("")) // Fail
			return 1
		}
		header := ""
		if useJsonRpcContentTypeHeader {
			header = "application/vscode-jsonrpc; charset=utf8"
		}
		client := jrpc2.NewClient(channel.Header(header)(out, in), &jrpc2.ClientOptions{})
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
		ctx := context.Background()
		jstr, _ := json.Marshal(params)
		log.Info("before call: ", fn, ",", params, ",jstr:", string(jstr))
		resp, err := client.Call(ctx, fn, params)
		log.Info("after call")
		if err != nil {
			log.Error("报错:" + err.Error())
			L.Push(lua.LString("")) // Resulting string
			return 1
		}
		L.Push(lua.LString(resp.ResultString())) // Resulting string
		return 1                                 // number of results
	}))

}

// LValue2SerialableValue LValue转可序列化的值
func LValue2SerialableValue(v *lua.LValue) interface{} {
	if v == nil {
		return nil
	}
	tp := (*v).Type()
	switch tp {
	case lua.LTTable:
		table := (*v).(*lua.LTable)
		result := make(map[string]interface{}, 0)
		table.ForEach(func(key, value lua.LValue) {
			result[key.String()] = LValue2SerialableValue(&value)
		})
		return result
	case lua.LTNil:
		return nil
	case lua.LTNumber, lua.LTBool, lua.LTString:
		return *v
	default:
		panic(fmt.Sprintf("LValue2Map()不支持的类型转换:%s", tp.String()))
	}
}
