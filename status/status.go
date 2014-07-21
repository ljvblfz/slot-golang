package status

import (
	"bytes"
	"encoding/json"
	"flag"
	"github.com/cuixin/atomic"
	glog "github.com/golang/glog"
	"net/http"
	"html/template"
	"os"
	"os/user"
	"runtime"
	"runtime/pprof"
	"time"
)

var (
	startTime	int64 // process start unixnano

	AppStat		*appStat
	running		atomic.AtomicBoolean

	//MsgStat = &MessageStat{}	// message
	//ConnStat = &ConnectionStat{}// connection
	//OLStat   = &OnlineStat{}	//
)

func init() {
	AppStat = &appStat{kv: make(map[string]*atomic.AtomicUint64)}
}

// 应用的具体信息
type appStat struct {
	kv map[string]*atomic.AtomicUint64
}

func (a *appStat) Add(key string) {
	if running.Get() {
		panic("Status server is running")
	}
	v := atomic.AtomicUint64(0)
	a.kv[key] = &v
}

func (a *appStat) Inc(key string) uint64 {
	return a.kv[key].Inc()
}

func (a *appStat) Dec(key string) uint64 {
	return a.kv[key].Dec()
}

func (a *appStat) Set(key string, value uint64) (old uint64) {
	old = a.kv[key].Set(value)
	return
}

func (a *appStat) Get(key string) uint64 {
	return a.kv[key].Get()
}

func (a *appStat) JsonBytes() []byte {
	res := map[string]interface{}{}
	for k, v := range a.kv {
		res[k] = v.Get()
	}
	return jsonRes(res)
}

func InitStat(addr string) {
	startTime = time.Now().UnixNano()
	glog.Infof("Start status server on addr:\"%s\"\n", addr)
	go func() {
		httpServeMux := http.NewServeMux()
		httpServeMux.HandleFunc("/stat", statHandle)
		running.Set(true)
		defer running.Set(false)
		if err := http.ListenAndServe(addr, httpServeMux); err != nil {
			glog.Errorf("http.ListenAdServe(\"%s\") error(%v)\n", addr, err)
			panic(err)
		}
	}()
}

// memory stats
func memStats() []byte {
	m := &runtime.MemStats{}
	runtime.ReadMemStats(m)
	// general
	res := map[string]interface{}{}
	res["alloc"] = m.Alloc
	res["total_alloc"] = m.TotalAlloc
	res["sys"] = m.Sys
	res["lookups"] = m.Lookups
	res["mallocs"] = m.Mallocs
	res["frees"] = m.Frees
	// heap
	res["heap_alloc"] = m.HeapAlloc
	res["heap_sys"] = m.HeapSys
	res["heap_idle"] = m.HeapIdle
	res["heap_inuse"] = m.HeapInuse
	res["heap_released"] = m.HeapReleased
	res["heap_objects"] = m.HeapObjects
	// low-level fixed-size struct alloctor
	res["stack_inuse"] = m.StackInuse
	res["stack_sys"] = m.StackSys
	res["mspan_inuse"] = m.MSpanInuse
	res["mspan_sys"] = m.MSpanSys
	res["mcache_inuse"] = m.MCacheInuse
	res["mcache_sys"] = m.MCacheSys
	res["buckhash_sys"] = m.BuckHashSys
	// GC
	res["next_gc"] = m.NextGC
	res["last_gc"] = m.LastGC
	res["pause_total_ns"] = m.PauseTotalNs
	res["pause_ns"] = m.PauseNs
	res["num_gc"] = m.NumGC
	res["enable_gc"] = m.EnableGC
	res["debug_gc"] = m.DebugGC
	res["by_size"] = m.BySize
	return jsonRes(res)
}

// golang stats
func goStats() []byte {
	res := map[string]interface{}{}
	res["compiler"] = runtime.Compiler
	res["arch"] = runtime.GOARCH
	res["os"] = runtime.GOOS
	res["max_procs"] = runtime.GOMAXPROCS(-1)
	res["root"] = runtime.GOROOT()
	res["cgo_call"] = runtime.NumCgoCall()
	res["goroutine_num"] = runtime.NumGoroutine()
	res["version"] = runtime.Version()
	return jsonRes(res)
}

func goroutineStats() []byte {
	buf := new(bytes.Buffer)
	if err := pprof.Lookup("goroutine").WriteTo(buf, 2); err != nil {
		return nil
	}
	return buf.Bytes()
}

// server stats
func serverStats() []byte {
	res := map[string]interface{}{}
	res["app_name"] = os.Args[0]
	res["running_time_ns"] = time.Now().UnixNano() - startTime
	hostname, _ := os.Hostname()
	res["hostname"] = hostname
	wd, _ := os.Getwd()
	res["wd"] = wd
	res["ppid"] = os.Getppid()
	res["pid"] = os.Getpid()
	res["pagesize"] = os.Getpagesize()
	if usr, err := user.Current(); err != nil {
		glog.Errorf("user.Current() error(%v)\n", err)
		res["group"] = ""
		res["user"] = ""
	} else {
		res["group"] = usr.Gid
		res["user"] = usr.Uid
	}
	return jsonRes(res)
}

// configuration info
func configInfo() []byte {
	res := map[string]interface{}{}
	flag.VisitAll(func(f *flag.Flag) {
		res[f.Name] = f.Value
	})
	return jsonRes(res)
}

// jsonRes format the output
func jsonRes(res map[string]interface{}) []byte {
	byteJson, err := json.MarshalIndent(res, "", "\t")
	if err != nil {
		glog.Errorf("json.MarshalIndent(\"%v\", \"\", \"    \") error(%v)\n", res, err)
		return nil
	}
	return byteJson
}

// statHandle get stat info by http
func statHandle(w http.ResponseWriter, r *http.Request) {
	if r.Method != "GET" {
		http.Error(w, "Method Not Allowed", 405)
		return
	}
	params := r.URL.Query()
	types := params.Get("type")
	auto := params.Get("refresh")
	res := []byte{}
	htmlRes := true

	switch types {
	case "":
		htmlRes = false
		w.Header().Add("Content-Type", "text/html")
		res = []byte(`<ul>
			<li><a href="/stat?type=memory">memory</a></li>
			<li><a href="/stat?type=server">server</a></li>
			<li><a href="/stat?type=golang">golang</a></li>
			<li><a href="/stat?type=goroutines">goroutines</a></li>
			<li><a href="/stat?type=config">config</a></li>
			<li><a href="/stat?type=app">online</a></li>
			<li><a href="/stat?type=app&refresh=auto">online(auto refresh)</a></li>
		</ul>`)
	case "memory":
		res = memStats()
	case "server":
		res = serverStats()
	case "golang":
		res = goStats()
	case "goroutines":
		htmlRes = false
		res = goroutineStats()
	case "config":
		res = configInfo()
	case "app":
		res = AppStat.JsonBytes()
	default:
		htmlRes = false
		http.Error(w, "Not Found", 404)
	}

	if auto == "auto" && htmlRes {
		w.Header().Add("Content-Type", "text/html; charset=utf-8")
		autoTemp.Execute(w, string(res))
	} else {
		if res != nil {
			if _, err := w.Write(res); err != nil {
				glog.Errorf("w.Write(\"%s\") error(%v)\n", string(res), err)
			}
		}
	}
}

var autoHtml = `
<html>
<body>
<pre>
{{.}}
</pre>
<script type="text/javascript">
setInterval("window.location.reload()", 2000);
</script>
</body>
</html>
`

var autoTemp = template.Must(template.New("autoRefresh").Parse(autoHtml))

//// Connection stat info
//type ConnectionStat struct {
//	Access atomic.AtomicUint64 // total access count
//	Create atomic.AtomicUint64 // total try login count
//	Delete atomic.AtomicUint64 // total failed login count
//}
//
//// Stat the connection info
//func (s *ConnectionStat) Stat() []byte {
//	res := map[string]interface{}{}
//	res["access"] = s.Access.Get()
//	res["create"] = s.Create.Get()
//	res["delete"] = s.Delete.Get()
//	return jsonRes(res)
//}
//
//// online user and goroutine count
//type OnlineStat struct {
//	Online atomic.AtomicUint64 // online users count
//}
//
//func (o *OnlineStat) Stat() []byte {
//	res := map[string]interface{}{}
//	res["online"] = o.Online.Get()
//	res["goroutine"] = runtime.NumGoroutine()
//	return jsonRes(res)
//}
//
//// Message stat info
//type MessageStat struct {
//	UpTimes       atomic.AtomicUint64 // total up coming message counts
//	UpFailedTimes atomic.AtomicUint64 // total up coming message failed counts
//	DownTimes     atomic.AtomicUint64 // total down going message counts
//	PubTimes      atomic.AtomicUint64 // pub messge count
//	PushTimes     atomic.AtomicUint64 // push messge count
//	SubTimes      atomic.AtomicUint64 // sub messge count
//}
//
//// Stat get the message stat info
//func (s *MessageStat) Stat() []byte {
//	res := map[string]interface{}{}
//	res["UpTimes"] = s.UpTimes.Get()
//	res["UpFailedTimes"] = s.UpFailedTimes.Get()
//	res["DownTimes"] = s.DownTimes.Get()
//	res["PubTimes"] = s.PubTimes.Get()
//	res["PushTimes"] = s.PushTimes.Get()
//	res["SubTimes"] = s.SubTimes.Get()
//	return jsonRes(res)
//}
