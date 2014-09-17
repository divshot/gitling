// Adapted from https://github.com/asim/git-http-backend (MIT License)
package server

import (
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"os/exec"
	"path"
	"regexp"
	"strconv"
	"strings"
	"time"
	
	"github.com/imdario/mergo"
)

type Service struct {
	Method  string
	Handler func(HandlerReq)
	Rpc     string
}

type Config struct {
  AuthURL     string
	ProjectRoot string
	GitBinPath  string
	Port        string
	UploadPack  bool
	ReceivePack bool
}

type AuthConfig struct {
  ReadOnly   bool `json:"read_only"`
  SkipCreate bool `json:"skip_create"`
}

type HandlerReq struct {
	w    http.ResponseWriter
	r    *http.Request
	Rpc  string
	Dir  string
	File string
}

var config = Config{
  ProjectRoot: "/home/mbleigh/sandbox/repos",
	GitBinPath:  "/usr/bin/git",
	Port:        "8080",
	UploadPack:  true,
	ReceivePack: true,
}

var services = map[string]Service{
	"(.*?)/git-upload-pack$":                       Service{"POST", serviceRpc, "upload-pack"},
	"(.*?)/git-receive-pack$":                      Service{"POST", serviceRpc, "receive-pack"},
	"(.*?)/info/refs$":                             Service{"GET", getInfoRefs, ""},
	"(.*?)/HEAD$":                                  Service{"GET", getTextFile, ""},
	"(.*?)/objects/info/alternates$":               Service{"GET", getTextFile, ""},
	"(.*?)/objects/info/http-alternates$":          Service{"GET", getTextFile, ""},
	"(.*?)/objects/info/packs$":                    Service{"GET", getInfoPacks, ""},
	"(.*?)/objects/info/[^/]*$":                    Service{"GET", getTextFile, ""},
	"(.*?)/objects/[0-9a-f]{2}/[0-9a-f]{38}$":      Service{"GET", getLooseObject, ""},
	"(.*?)/objects/pack/pack-[0-9a-f]{40}\\.pack$": Service{"GET", getPackFile, ""},
	"(.*?)/objects/pack/pack-[0-9a-f]{40}\\.idx$":  Service{"GET", getIdxFile, ""},
}

// Request handling function

func requestHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		log.Printf("%s %s %s %s", r.RemoteAddr, r.Method, r.URL.Path, r.Proto)
		
		for match, service := range services {
			re, err := regexp.Compile(match)
			if err != nil {
				log.Print(err)
			}

			if m := re.FindStringSubmatch(r.URL.Path); m != nil {
				if service.Method != r.Method {
					renderMethodNotAllowed(w, r)
					return
				}

				rpc := service.Rpc
				file := strings.Replace(r.URL.Path, m[1]+"/", "", 1)
				
  		  authResponse, err := AuthorizeRequest(m[1], w, r, config)
  		  if err != nil {
  		    authErr, ok := err.(AuthError)
  		    if ok {
  		      renderAuthError(w, authErr)
  		    }
  		    log.Println(err)
  		    return
  		  }
				
				dir, err := getGitDir(authResponse.Path)

				if err != nil {
					log.Print(err)
					renderNotFound(w)
					return
				}

				hr := HandlerReq{w, r, rpc, dir, file}
				service.Handler(hr)
				return
			}
		}
		renderNotFound(w)
		return
	}
}

// Actual command handling functions

func serviceRpc(hr HandlerReq) {
	w, r, rpc, dir := hr.w, hr.r, hr.Rpc, hr.Dir
	access := hasAccess(r, dir, rpc, true)

	if access == false {
		renderNoAccess(w)
		return
	}

	input, _ := ioutil.ReadAll(r.Body)

	w.Header().Set("Content-Type", fmt.Sprintf("application/x-git-%s-result", rpc))
	w.WriteHeader(http.StatusOK)

	args := []string{rpc, "--stateless-rpc", dir}
	cmd := exec.Command(config.GitBinPath, args...)
	cmd.Dir = dir
	in, err := cmd.StdinPipe()
	if err != nil {
		log.Print(err)
	}

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		log.Print(err)
	}

	err = cmd.Start()
	if err != nil {
		log.Print(err)
	}

	in.Write(input)
	io.Copy(w, stdout)
	cmd.Wait()
}

func getInfoRefs(hr HandlerReq) {
	w, r, dir := hr.w, hr.r, hr.Dir
	service_name := getServiceType(r)
	access := hasAccess(r, dir, service_name, false)

	if access {
		args := []string{service_name, "--stateless-rpc", "--advertise-refs", "."}
		refs := gitCommand(dir, args...)

		hdrNocache(w)
		w.Header().Set("Content-Type", fmt.Sprintf("application/x-git-%s-advertisement", service_name))
		w.WriteHeader(http.StatusOK)
		w.Write(packetWrite("# service=git-" + service_name + "\n"))
		w.Write(packetFlush())
		w.Write(refs)
	} else {
		updateServerInfo(dir)
		hdrNocache(w)
		sendFile("text/plain; charset=utf-8", hr)
	}
}

func getInfoPacks(hr HandlerReq) {
	hdrCacheForever(hr.w)
	sendFile("text/plain; charset=utf-8", hr)
}

func getLooseObject(hr HandlerReq) {
	hdrCacheForever(hr.w)
	sendFile("application/x-git-loose-object", hr)
}

func getPackFile(hr HandlerReq) {
	hdrCacheForever(hr.w)
	sendFile("application/x-git-packed-objects", hr)
}

func getIdxFile(hr HandlerReq) {
	hdrCacheForever(hr.w)
	sendFile("application/x-git-packed-objects-toc", hr)
}

func getTextFile(hr HandlerReq) {
	hdrNocache(hr.w)
	sendFile("text/plain", hr)
}

// Logic helping functions

func sendFile(content_type string, hr HandlerReq) {
	w, r := hr.w, hr.r
	req_file := path.Join(hr.Dir, hr.File)

	f, err := os.Stat(req_file)
	if os.IsNotExist(err) {
		renderNotFound(w)
		return
	}

	w.Header().Set("Content-Type", content_type)
	w.Header().Set("Content-Length", fmt.Sprintf("%d", f.Size()))
	w.Header().Set("Last-Modified", f.ModTime().Format(http.TimeFormat))
	http.ServeFile(w, r, req_file)
}

func getGitDir(file_path string) (string, error) {
	root := config.ProjectRoot

	if root == "" {
		cwd, err := os.Getwd()

		if err != nil {
			log.Print(err)
			return "", err
		}

		root = cwd
	}

	f := path.Join(root, file_path)
	if _, err := os.Stat(f); os.IsNotExist(err) {
		return "", err
	}

	return f, nil
}

func getServiceType(r *http.Request) string {
	service_type := r.FormValue("service")

	if s := strings.HasPrefix(service_type, "git-"); !s {
		return ""
	}

	return strings.Replace(service_type, "git-", "", 1)
}

func hasAccess(r *http.Request, dir string, rpc string, check_content_type bool) bool {
	if check_content_type {
		if r.Header.Get("Content-Type") != fmt.Sprintf("application/x-git-%s-request", rpc) {
			return false
		}
	}

	if !(rpc == "upload-pack" || rpc == "receive-pack") {
		return false
	}
	if rpc == "receive-pack" {
		return config.ReceivePack
	}
	if rpc == "upload-pack" {
		return config.UploadPack
	}

	return getConfigSetting(rpc, dir)
}

func getConfigSetting(service_name string, dir string) bool {
	service_name = strings.Replace(service_name, "-", "", -1)
	setting := getGitConfig("http."+service_name, dir)

	if service_name == "uploadpack" {
		return setting != "false"
	}

	return setting == "true"
}

func getGitConfig(config_name string, dir string) string {
	args := []string{"config", config_name}
	out := string(gitCommand(dir, args...))
	return out[0 : len(out)-1]
}

func updateServerInfo(dir string) []byte {
	args := []string{"update-server-info"}
	return gitCommand(dir, args...)
}

func gitCommand(dir string, args ...string) []byte {
	command := exec.Command(config.GitBinPath, args...)
	command.Dir = dir
	out, err := command.Output()

	if err != nil {
		log.Print(err)
	}

	return out
}

// HTTP error response handling functions

func renderMethodNotAllowed(w http.ResponseWriter, r *http.Request) {
	if r.Proto == "HTTP/1.1" {
		w.WriteHeader(http.StatusMethodNotAllowed)
		w.Write([]byte("Method Not Allowed"))
	} else {
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte("Bad Request"))
	}
}

func renderNotFound(w http.ResponseWriter) {
	w.WriteHeader(http.StatusNotFound)
	w.Write([]byte("Not Found"))
}

func renderNoAccess(w http.ResponseWriter) {
	w.WriteHeader(http.StatusForbidden)
	w.Write([]byte("Forbidden"))
}

func renderAuthError(w http.ResponseWriter, err AuthError) {
  w.WriteHeader(err.Status)
  w.Write([]byte(err.Message))
}

// Packet-line handling function

func packetFlush() []byte {
	return []byte("0000")
}

func packetWrite(str string) []byte {
	s := strconv.FormatInt(int64(len(str)+4), 16)

	if len(s)%4 != 0 {
		s = strings.Repeat("0", 4-len(s)%4) + s
	}

	return []byte(s + str)
}

// Header writing functions

func hdrNocache(w http.ResponseWriter) {
	w.Header().Set("Expires", "Fri, 01 Jan 1980 00:00:00 GMT")
	w.Header().Set("Pragma", "no-cache")
	w.Header().Set("Cache-Control", "no-cache, max-age=0, must-revalidate")
}

func hdrCacheForever(w http.ResponseWriter) {
	now := time.Now().Unix()
	expires := now + 31536000
	w.Header().Set("Date", fmt.Sprintf("%d", now))
	w.Header().Set("Expires", fmt.Sprintf("%d", expires))
	w.Header().Set("Cache-Control", "public, max-age=31536000")
}

// Main
func Start(c Config) {
  mergo.Merge(&config, c)
	http.HandleFunc("/", requestHandler())

  println("** Starting HTTP server on port", config.Port)
	err := http.ListenAndServe(":" + config.Port, nil)
	if err != nil {
		log.Fatal("ListenAndServe: ", err)
	}
}
