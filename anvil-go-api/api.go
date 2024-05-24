package api

import (
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"os"

	"github.com/ddkwork/golibrary/mylog"
)

func checkHttpError(rsp *http.Response, msg string) error {
	if rsp.StatusCode < 200 || rsp.StatusCode >= 300 {
		body, _ := ioutil.ReadAll(rsp.Body)
		return fmt.Errorf("%s: response contained a non-success status code (%d) %s\n", msg, rsp.StatusCode, string(body))
	}
	return nil
}

func prefixError(err error, msg string) error {
	if err == nil {
		return nil
	}

	return fmt.Errorf("%s: %s", msg, err.Error())
}

type URLs struct {
	base string
}

func NewURLs(port string) URLs {
	return URLs{
		base: fmt.Sprintf("http://localhost:%s", port),
	}
}

func (u URLs) Build(path string) string {
	return fmt.Sprintf("%s%s", u.base, path)
}

type Anvil struct {
	sessId string
	urls   URLs
	client http.Client
}

func New(sessId, port string) Anvil {
	return Anvil{
		sessId: sessId,
		urls:   NewURLs(port),
	}
}

func NewFromEnv() (anvil Anvil, err error) {
	sessId := os.Getenv("ANVIL_API_SESS")
	port := os.Getenv("ANVIL_API_PORT")

	if sessId == "" {
		mylog.Check(fmt.Errorf("environment variable ANVIL_API_SESS is not set"))
		return
	}
	if port == "" {
		mylog.Check(fmt.Errorf("environment variable ANVIL_API_PORT is not set"))
		return
	}

	anvil = Anvil{
		sessId: sessId,
		urls:   NewURLs(port),
	}
	return
}

func (a Anvil) Get(path string) (rsp *http.Response, err error) {
	req, url := mylog.Check3(a.buildReq(http.MethodGet, path, nil))

	rsp = mylog.Check2(a.client.Do(req))
	mylog.Check(prefixError(err, fmt.Sprintf("GET to %s failed", url)))
	mylog.Check(checkHttpError(rsp, fmt.Sprintf("GET to %s failed", url)))
	return
}

func (a Anvil) GetInto(path string, resp interface{}) (err error) {
	rsp := mylog.Check2(a.Get(path))
	raw := mylog.Check2(ioutil.ReadAll(rsp.Body))
	mylog.Check(prefixError(err, "Error reading body info"))
	mylog.Check(json.Unmarshal(raw, resp))
	mylog.Check(prefixError(err, fmt.Sprintf("Error decoding JSON GET response body, body is '%s'", raw)))
	return
}

func (a Anvil) Post(path string, body io.Reader) (rsp *http.Response, err error) {
	req, url := mylog.Check3(a.buildReq(http.MethodPost, path, body))

	rsp = mylog.Check2(a.client.Do(req))
	mylog.Check(prefixError(err, fmt.Sprintf("POST to %s failed", url)))
	mylog.Check(checkHttpError(rsp, fmt.Sprintf("POST to %s failed", url)))
	return
}

func (a Anvil) Put(path string, body io.Reader) (rsp *http.Response, err error) {
	req, url := mylog.Check3(a.buildReq(http.MethodPut, path, body))

	rsp = mylog.Check2(a.client.Do(req))
	mylog.Check(prefixError(err, fmt.Sprintf("PUT to %s failed", url)))
	mylog.Check(checkHttpError(rsp, fmt.Sprintf("PUT to %s failed", url)))
	return
}

func (a Anvil) buildReq(method, path string, body io.Reader) (req *http.Request, url string, err error) {
	url = a.urls.Build(path)
	req = mylog.Check2(http.NewRequest(method, url, body))
	mylog.Check(prefixError(err, fmt.Sprintf("Error building %s request for %s", method, url)))

	req.Header.Add("Anvil-Sess", a.sessId)
	req.Header.Add("Content-Type", "application/json")
	req.Header.Add("Accept", "application/json")
	return
}
