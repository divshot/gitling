package server

import (
  "encoding/base64"
  "encoding/json"
  "net/http"
  "strings"
  "errors"
  // "log"
  "time"
  "fmt"
  
  "github.com/franela/goreq"
)

type AuthError struct {
  Status int `json:"status"`
  Message string `json:"error_description"`
}

func (e AuthError) Error() string {
  return fmt.Sprintf("%d: %s", e.Status, e.Message)
}

type AuthResponse struct {
  Path string
  ReadOnly bool
  SkipCreate bool
}

func AuthorizeRequest(path string, w http.ResponseWriter, r *http.Request, config Config) (result AuthResponse, err error) {
  if config.AuthURL == "" {
    result = AuthResponse{path, false, false}
    return
  }
  
  username, password, ok := basicAuth(r)
  if !ok {
    err = errors.New("No HTTP credentials were supplied. Issuing challenge.")
    writeChallenge(w)
    return
  }
  
  reqBody, err := json.Marshal(map[string]string{"username":username, "password":password, "path":path})
  if err != nil {
    println(err)
    return
  }
  println(string(reqBody[:]))
  
  req := goreq.Request{
    Method: "POST",
    Uri: config.AuthURL,
    ContentType: "application/json",
    Accept: "application/json",
    Body: reqBody,
    Timeout: 3000 * time.Millisecond,
  }
  
  res, err := req.Do()
  if err != nil {
    return
  }
  
  switch res.StatusCode{
    case 200,201:
      result = AuthResponse{"/custom/path", true, true}
    case 204:
      result = AuthResponse{path, false, false} 
    case 401, 403:
      body, er := res.Body.ToString()
      if er != nil {
        body = "Unauthorized"
      }
      
      err = AuthError{res.StatusCode, body}
    case 404:
      err = AuthError{res.StatusCode, "The repository could not be found"}
    default:
      err = AuthError{res.StatusCode, "An unknown error occurred"}
  }
  
  return
}

func writeChallenge(w http.ResponseWriter) {
  w.Header().Set("WWW-Authenticate", "Basic realm=\"git\"")
  w.WriteHeader(401)
  w.Write([]byte("Unauthorized"))
}

// BasicAuth returns the username and password provided in the request's
// Authorization header, if the request uses HTTP Basic Authentication.
// See RFC 2617, Section 2.
func basicAuth(r *http.Request) (username, password string, ok bool) {
	auth := r.Header.Get("Authorization")
	if auth == "" {
		return
	}
	return parseBasicAuth(auth)
}

// parseBasicAuth parses an HTTP Basic Authentication string.
// "Basic QWxhZGRpbjpvcGVuIHNlc2FtZQ==" returns ("Aladdin", "open sesame", true).
func parseBasicAuth(auth string) (username, password string, ok bool) {
	if !strings.HasPrefix(auth, "Basic ") {
		return
	}
	c, err := base64.StdEncoding.DecodeString(strings.TrimPrefix(auth, "Basic "))
	if err != nil {
		return
	}
	cs := string(c)
	s := strings.IndexByte(cs, ':')
	if s < 0 {
		return
	}
	return cs[:s], cs[s+1:], true
}