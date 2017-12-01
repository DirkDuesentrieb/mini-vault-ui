// Copyright 2017 github.com/DirkDuesentrieb
// license that can be found in the LICENSE file.

// Simple WebUI for Vault

package main

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"html/template"
	"io/ioutil"
	"net/http"
	"net/url"
	"os/exec"
	"runtime"
	"strings"
	"time"
)

const (
	listt string = `<!DOCTYPE html>
	<html>
		<head>
			<meta charset="UTF-8">
			<title>Mini Vault UI</title>
		</head>
		<body>
			<h1>vault list {{.Location}}</h1>
			{{$l := .Location}}
			<table>
				{{range .Paths}}
					<tr>
					<td><a href="/list/{{$l}}/{{.}}">{{.}}/</a></td><td></td>
					</tr>
					{{end}}
				{{range .Secrets}}
					<tr>
					<td><a href="/read/{{$l}}/{{.}}">{{.}}</a></td><td>[<a href="/delete/{{$l}}/{{.}}">del</a>]</td>
					</tr>
			{{end}}
			<tr>
				<form action="/new/{{$l}}">
				<td><input type="text" name="secret" size=30 value="new secret"></input></td><td><button>new</button></td>
			</tr>
			</form>
			</table>
			<br>
			<br>[<a href="/list/{{.Up}}">back</a>] [<a href="/setting/list/{{$l}}">Settings</a>]<br>
		</body>
	</html>`
	indext string = `<!DOCTYPE html>
	<html>
		<head>
			<meta charset="UTF-8">
			<title>Mini Vault UI</title>
		</head>
		<body>
			<h1>Mini Vault UI</h1>
			<a href="/list/secret">/list/secret</a><br>
			[<a href="/setting/">Settings</a>]
		</body>
	</html>`
	readt string = `<!DOCTYPE html>
	<html>
		<head>
			<meta charset="UTF-8">
			<title>Mini Vault UI</title>
		</head>
		<body>
			<h1>vault read {{.Location}}</h1>
			<h2>As Key/Value</h2>
			{{$l := .Location}}
			<form action="/write/{{$l}}">
			<table>
				<tr><td>KEY</td><td>VALUE</td></tr>
			{{range .Attr}}
				<tr>
				  <td><input type="text" name="k{{.Idx}}" size=40 value="{{.Key}}"></input></td>
				  <td><input type="text" name="vk{{.Idx}}" size=40 value="{{.Value}}"></input></td>
				</tr>
			{{end}}
			<tr>
			  <td><input type="text" name="k" size=40></input></td>
			  <td><input type="text" name="vk" size=40></input></td>
			</tr>
			</table>
			[<a href="/list/{{.Up}}">back</a>]  <button>write</button>
			</form>
			<h2>As JSON</h2>
			<form action="/writejson/{{$l}}">
			<textarea name="json" rows="5" cols="90">{{.JSON}}</textarea>
			<br>
			[<a href="/list/{{.Up}}">back</a>] <button>write json</button>
			</form>
		</body>
	</html>`
	settingt string = `<!DOCTYPE html>
	<html>
		<head>
			<meta charset="UTF-8">
			<title>Mini Vault UI</title>
		</head>
		<body>
			<h1>Mini Vault UI</h1>
			{{if .LastErr}}
			<b>{{.LastErr}}</b><br>
			{{end}}
			<h2>Edit Settings</h2>
			<form action="/set/{{.Redir}}">
			<table>
			<tr><td>Server URL</td><td><input type="text" name="srv" size=40 value="{{.Srv}}"></input></td></tr>
			<tr><td>
			<label>Login with</td><td>
	        <select name="login">
	          <option value="ldap">LDAP</option>
	          <option {{if .Tok}} selected {{end}} value="token">API Token</option>
	        </select>
	      </label></td></tr>
			<tr><td>Username</td><td><input type="text" name="user" size=40 value="{{.User}}"></input></td></tr>
			<tr><td>Password</td><td><input type="password" name="pass" size=40 ></input></td></tr>
			<tr><td>or</td><td></td></tr>
			<tr><td>Token</td><td><input type="text" name="tok" size=40 value="{{.Tok}}"></input></td></tr>
			</table>
			<button>save</button>
			</form>
		</body>
	</html>`
)

var (
	listT     = template.Must(template.New("list.html").Parse(listt))
	indexT    = template.Must(template.New("index.html").Parse(indext))
	readT     = template.Must(template.New("read.html").Parse(readt))
	settingT  = template.Must(template.New("setting.html").Parse(settingt))
	port      string
	startPath string
	lasterr   string = ""
)

type (
	// structs to fill the html templates
	settingTempl struct {
		Srv     string
		Tok     string
		User    string
		LastErr string
		Redir   string
	}
	readTempl struct {
		Attr     []kv
		JSON     string
		Location string
		Up       string
	}
	listTempl struct {
		Paths    []string
		Secrets  []string
		Location string
		Up       string
		LastErr  string
	}
	kv struct {
		Key   string
		Value string
		Idx   int // needed in template
	}
	// key/value data object (ignore the rest)
	kvdata struct {
		Data   map[string]string `json:"data"`
		Errors []string          `json:"errors"`
	}
	// list object with "keys" array
	keysdata struct {
		Data   keys     `json:"data"`
		Errors []string `json:"errors"`
	}
	keys struct {
		Keys []string `json:"keys"`
	}
)

func main() {
	flag.StringVar(&startPath, "s", "/list/secret/", "Start with this path (Windows only)")
	flag.StringVar(&port, "p", "7777", "Listening port")
	flag.Parse()

	http.HandleFunc("/setting/", settingHandler)
	http.HandleFunc("/set/", setHandler)
	http.HandleFunc("/", genericHandler)
	fmt.Println("Listening on http://localhost:" + port)
	if runtime.GOOS == "windows" {
		cmd := exec.Command("C:\\WINDOWS\\system32\\cmd.exe", "/k", "start", "http://localhost:"+port + startPath)
		err := cmd.Run()
		myErr("cmd.Run()", err)
	}
	err := http.ListenAndServe("127.0.0.1:"+port, nil)
	myErr("http.ListenAndServe()", err)
}

// Settings
func settingHandler(w http.ResponseWriter, r *http.Request) {
	location := r.URL.Path[len("/setting/"):]
	vtok := ""
	vsrv := ""
	vuser := ""
	vcookie, err := r.Cookie("vtoken")
	if err == nil {
		vtok = vcookie.Value
	}
	vcookie, err = r.Cookie("vserver")
	if err == nil {
		vsrv = vcookie.Value
	}
	vcookie, err = r.Cookie("vuser")
	if err == nil {
		vuser = vcookie.Value
	}
	myTempl := settingTempl{vsrv, vtok, vuser, lasterr, location}
	err = settingT.Execute(w, myTempl)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
}

// set cookies
func setHandler(w http.ResponseWriter, r *http.Request) {
	var token, errtext string
	var err error
	var dursec int
	var dur time.Duration
	dur, _ = time.ParseDuration("4h")
	location := r.URL.Path[len("/set/"):]
	expiration := time.Now()
	ps, _ := url.ParseQuery(r.URL.RawQuery)
	//fmt.Println("tok=",ps["tok"][0],"srv=",ps["srv"][0],"login=",ps["login"][0])
	scookie := http.Cookie{Name: "vserver", Value: ps["srv"][0], Expires: expiration.Add(dur), Path: "/"}
	http.SetCookie(w, &scookie)
	if ps["login"][0] == "ldap" {
		ucookie := http.Cookie{Name: "vuser", Value: ps["user"][0], Expires: expiration.Add(dur), Path: "/"}
		http.SetCookie(w, &ucookie)
		token, dursec, err, errtext = vLdapLogin(ps["srv"][0], ps["user"][0], ps["pass"][0])
		lasterr = errtext
		if err != nil {
			http.Redirect(w, r, "/setting/", http.StatusFound)
			//http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		dur = time.Duration(int64(dursec * 1000000000))
	} else {
		token = ps["tok"][0]
	}
	//fmt.Println("token=", token, "dur=", dur, "lasterr=", errtext)
	tcookie := http.Cookie{Name: "vtoken", Value: token, Expires: expiration.Add(dur), Path: "/"}
	http.SetCookie(w, &tcookie)
	http.Redirect(w, r, "/"+location, http.StatusFound)
}

// generic router
func genericHandler(w http.ResponseWriter, r *http.Request) {
	cmds := strings.Split(r.URL.Path, "/")
	cmd := ""
	location := ""
	if len(cmds) > 2 {
		cmd = cmds[1]
		location = r.URL.Path[len(cmd)+2:]
	}
	tok, srv, err := checkCookies(r)
	if err != nil {
		lasterr = "Unauthorized"
		http.Redirect(w, r, "/setting/"+r.URL.Path, http.StatusFound)
		return
	}
	//fmt.Println("cmd=", cmd, "location=", location)
	switch cmd {

	case "read":
		var read kvdata
		vOut, code, err := vApi(srv+"/v1/"+location, tok)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		if code == 404 {
			http.Redirect(w, r, "/list/"+upperDir(location), http.StatusFound)
			return
		}
		err = json.Unmarshal(vOut, &read)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		newItem := make([]kv, len(read.Data))
		i := 0
		for k, v := range read.Data {
			newItem[i] = kv{k, v, i}
			i++
		}
		js, _ := json.Marshal(read.Data)

		myTempl := readTempl{newItem, string(js), location, upperDir(location)}
		err = readT.Execute(w, myTempl)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

	case "writejson":
		ps, _ := url.ParseQuery(r.URL.RawQuery)
		err = updateJSON(ps["json"][0], srv+"/v1/"+location, tok)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		http.Redirect(w, r, "/read/"+location, http.StatusFound)

	case "delete":
		client := &http.Client{}
		req, _ := http.NewRequest("DELETE", srv+"/v1/"+location, nil)
		req.Header.Add("X-Vault-Token", tok)
		_, err = client.Do(req)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		http.Redirect(w, r, "/list/"+upperDir(location), http.StatusFound)

	case "new":
		ps, _ := url.ParseQuery(r.URL.RawQuery)
		err = updateJSON("{\"new key\": \"new value\"}", srv+"/v1/"+location+"/"+ps["secret"][0], tok)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		http.Redirect(w, r, "/read/"+location+"/"+ps["secret"][0], http.StatusFound)

	case "list":
		secrets := []string{}
		paths := []string{}

		vOut, code, err := vApi(srv+"/v1/"+location+"/?list=true", tok)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		if code == 404 {
			//http.Error(w, location + " ist not a valid Path!", http.StatusNotFound)
			//http.Redirect(w, r, "/list/"+upperDir(location), http.StatusFound)
			http.Redirect(w, r, "/", http.StatusFound)
			return
		}
		var kd keysdata
		err = json.Unmarshal(vOut, &kd)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		if len(kd.Errors) > 0 {
			//lasterr = kd.Errors[0]
			http.Error(w, "list "+location+" "+kd.Errors[0], http.StatusNotFound)
			//http.Redirect(w, r, "/setting/", http.StatusFound)
			return
		}
		for i := 0; i < len(kd.Data.Keys); i++ {
			a := kd.Data.Keys[i]
			// secret or path?
			if a[len(a)-1:] == "/" {
				paths = append(paths, a[:len(a)-1])
			} else {
				secrets = append(secrets, a)
			}
		}
		myTempl := listTempl{paths, secrets, location, upperDir(location), lasterr}

		err = listT.Execute(w, myTempl)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		lasterr = ""

	case "write":
		ps, _ := url.ParseQuery(r.URL.RawQuery)

		// crude way to create a JSON from k/v pairs
		kv := "{"
		secret := []string{}
		for k, karr := range ps {
			if karr[0] != "" {
				if k[:1] == "k" {
					varr, exists := ps["v"+k]
					if exists {
						kv += "\"" + karr[0] + "\": " + "\"" + varr[0] + "\","
						secret = append(secret, kv)
					}
				}
			}
		}
		kv = kv[:len(kv)-1]
		kv += "}"
		err = updateJSON(kv, srv+"/v1/"+location, tok)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		http.Redirect(w, r, "/read/"+location, http.StatusFound)

	case "":
		err = indexT.Execute(w, nil)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
	default:
		http.Redirect(w, r, "/", http.StatusFound)
	}
}

// Login with LDAP
func vLdapLogin(srv, user, password string) (token string, dur int, err error, errtext string) {
	type lrAuth struct {
		Token string `json:"client_token"`
		Dur   int    `json:"lease_duration"`
	}
	type loginresp struct {
		Auth   lrAuth   `json:"auth"`
		Errors []string `json:"errors"`
	}
	lr := loginresp{}
	passjson := "{ \"password\": \"" + password + "\" }"
	client := &http.Client{}
	req, err := http.NewRequest("POST", srv+"/v1/auth/ldap/login/"+user, strings.NewReader(passjson))
	req.Header.Add("Content-Type", "application/json")
	resp, err := client.Do(req)
	if err != nil {
		return "", 0, err, err.Error()
	}
	vOut, _ := ioutil.ReadAll(resp.Body)
	err = json.Unmarshal(vOut, &lr)
	if err != nil {
		return "", 0, err, err.Error()
	}
	code := resp.StatusCode
	if code != 200 {
		for _, v := range lr.Errors {
			errtext += string(v) + "\n"
		}
		return "", 0, errors.New(errtext), errtext
		//errtext = lr.Errors[0]
	}
	return lr.Auth.Token, lr.Auth.Dur, nil, ""
}

// write secret at <location> with JSON data
func updateJSON(jsn, url, token string) error {
	client := &http.Client{}
	req, err := http.NewRequest("POST", url, strings.NewReader(jsn))
	myErr("http.NewRequest()", err)
	req.Header.Add("X-Vault-Token", token)
	req.Header.Add("Content-Type", "application/json")
	_, err = client.Do(req)
	if err != nil {
		return err
	}
	return nil
}

// cut last part from path
func upperDir(location string) string {
	i := len(location)
	for ; location[i-1:i] != "/" && i > 1; i-- {
	}
	return location[:i-1]
}

// simple wrapper to vaults http API
func vApi(url, token string) (vOut []byte, code int, err error) {
	client := &http.Client{}
	req, err := http.NewRequest("GET", url, nil)
	myErr("http.NewRequest()", err)
	req.Header.Add("X-Vault-Token", token)
	resp, err := client.Do(req)
	myErr("client.Do()", err)
	code = resp.StatusCode
	vOut, err = ioutil.ReadAll(resp.Body)
	myErr("ioutil.ReadAll()", err)
	return
}

// test if the client has the cookies
func checkCookies(r *http.Request) (tok string, srv string, err error) {
	vcookie, err := r.Cookie("vtoken")
	if err != nil {
		return
	}
	tok = vcookie.Value

	vcookie, err = r.Cookie("vserver")
	if err != nil {
		return
	}
	srv = vcookie.Value
	return
}

func myErr(desc string, err error) {
	if err != nil {
		fmt.Println("Error in "+desc+":", err.Error())
	}
}
