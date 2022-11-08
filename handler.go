package main

import (
	"fmt"
	"html/template"
	"log"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"
)

type wasmServer struct {
	indexTmpl  *template.Template
	wasmFile   string
	wasmExecJS []byte
	args       []string
	envMap     map[string]string
	logger     *log.Logger
}

func wasmExecJS() string {
	wasmExecJS := os.Getenv("WASM_EXECJS")
	if len(wasmExecJS) > 0 {
		if fi, err := os.Stat(wasmExecJS); err != nil || fi.IsDir() {
			fmt.Printf("can not find %s, use default: %v\n", wasmExecJS, err)
			goto WASM_JS
		} else {
			fmt.Printf("use %s\n", wasmExecJS)
			return wasmExecJS
		}
	}

WASM_JS:
	rootFolder := os.Getenv("TINYGOROOT")

	if len(rootFolder) > 0 {
		return path.Join(rootFolder, "targets/wasm_exec.js")
	}

	return path.Join(runtime.GOROOT(), "misc/wasm/wasm_exec.js")
}

func loadIndex() string {
	indexFile := os.Getenv("WASM_INDEX")
	if len(indexFile) > 0 {
		content, err := os.ReadFile(indexFile)
		if err != nil {
			fmt.Println("unable to read index file and use default:", err)
			return indexHTMLDefault
		}
		return string(content)
	}

	return indexHTMLDefault
}

func NewWASMServer(wasmFile string, args []string, l *log.Logger) (http.Handler, error) {
	var err error
	srv := &wasmServer{
		wasmFile: wasmFile,
		args:     args,
		logger:   l,
		envMap:   make(map[string]string),
	}

	for _, env := range os.Environ() {
		vars := strings.SplitN(env, "=", 2)
		srv.envMap[vars[0]] = vars[1]
	}

	buf, err := os.ReadFile(wasmExecJS())
	if err != nil {
		return nil, err
	}
	srv.wasmExecJS = buf

	srv.indexTmpl, err = template.New("index").Parse(loadIndex())
	if err != nil {
		return nil, err
	}
	return srv, nil
}

func (ws *wasmServer) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	// log.Println(r.URL.Path)
	switch r.URL.Path {
	case "/", "/index.html":
		w.Header().Set("Content-Type", "text/html; charset=UTF-8")
		data := struct {
			WASMFile string
			Args     []string
			EnvMap   map[string]string
		}{
			WASMFile: filepath.Base(ws.wasmFile),
			Args:     ws.args,
			EnvMap:   ws.envMap,
		}
		err := ws.indexTmpl.Execute(w, data)
		if err != nil {
			ws.logger.Println(err)
		}
	case "/" + filepath.Base(ws.wasmFile):
		f, err := os.Open(ws.wasmFile)
		if err != nil {
			ws.logger.Println(err)
			return
		}
		defer func() {
			err := f.Close()
			if err != nil {
				ws.logger.Println(err)
			}
		}()
		http.ServeContent(w, r, r.URL.Path, time.Now(), f)
	case "/wasm_exec.js":
		w.Header().Set("Content-Type", "application/javascript")
		w.Header().Set("Content-Length", strconv.Itoa(len(ws.wasmExecJS)))
		if _, err := w.Write(ws.wasmExecJS); err != nil {
			ws.logger.Println("unable to write wasm_exec.")
		}
	}
}

const indexHTMLDefault = `<!doctype html>
<!--
Copyright 2018 The Go Authors. All rights reserved.
Use of this source code is governed by a BSD-style
license that can be found in the LICENSE file.
-->
<html>

<head>
	<meta charset="utf-8">
	<title>Go wasm</title>
</head>

<body>
	<!--
	Add the following polyfill for Microsoft Edge 17/18 support:
	<script src="https://cdn.jsdelivr.net/npm/text-encoding@0.7.0/lib/encoding.min.js"></script>
	(see https://caniuse.com/#feat=textencoder)
	-->
	<script src="wasm_exec.js"></script>
	<script>
		if (!WebAssembly.instantiateStreaming) { // polyfill
			WebAssembly.instantiateStreaming = async (resp, importObject) => {
				const source = await (await resp).arrayBuffer();
				return await WebAssembly.instantiate(source, importObject);
			};
		}

		let exitCode = 0;
		function goExit(code) {
			exitCode = code;
		}

		(async() => {
			const go = new Go();
			go.argv = [{{range $i, $item := .Args}} {{if $i}}, {{end}} "{{$item}}" {{end}}];
			// The notFirst variable sets itself to true after first iteration. This is to put commas in between.
			go.env = { {{ $notFirst := false }}
			{{range $key, $val := .EnvMap}} {{if $notFirst}}, {{end}} {{$key}}: "{{$val}}" {{ $notFirst = true }}
			{{end}} };
			go.exit = goExit;
			let mod, inst;
			await WebAssembly.instantiateStreaming(fetch("{{.WASMFile}}"), go.importObject).then((result) => {
				mod = result.module;
				inst = result.instance;
			}).catch((err) => {
				console.error(err);
			});
			await go.run(inst);
			document.getElementById("doneButton").disabled = false;
		})();
	</script>

	<button id="doneButton" style="display: none;" disabled>Done</button>
</body>
</html>`

/*
<!doctype html>
<!--
Copyright 2018 The Go Authors. All rights reserved.
Use of this source code is governed by a BSD-style
license that can be found in the LICENSE file.
-->
<html>

<head>
	<meta charset="utf-8">
	<title>Go wasm</title>
</head>

<body>
	<!--
	Add the following polyfill for Microsoft Edge 17/18 support:
	<script src="https://cdn.jsdelivr.net/npm/text-encoding@0.7.0/lib/encoding.min.js"></script>
	(see https://caniuse.com/#feat=textencoder)
	-->
	<script src="wasm_exec.js"></script>
	<script>
		if (!WebAssembly.instantiateStreaming) { // polyfill
			WebAssembly.instantiateStreaming = async (resp, importObject) => {
				const source = await (await resp).arrayBuffer();
				return await WebAssembly.instantiate(source, importObject);
			};
		}

		const go = new Go();
		let mod, inst;
		WebAssembly.instantiateStreaming(fetch("wasm.wasm"), go.importObject).then((result) => {
			mod = result.module;
			inst = result.instance;
			document.getElementById("runButton").disabled = false;
		}).catch((err) => {
			console.error(err);
		});

		async function run() {
			console.clear();
			await go.run(inst);
			inst = await WebAssembly.instantiate(mod, go.importObject); // reset instance
		}
	</script>

	<button onClick="run();" id="runButton" disabled>Run</button>
</body>

</html>
*/
