package githubaction_test

import (
	"bytes"
	"encoding/json"
	"io"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"testing"

	"github.com/wader/bump/internal/bump"
	"github.com/wader/bump/internal/github"
	"github.com/wader/bump/internal/githubaction"
)

func TestCheckTemplateReplaceFn(t *testing.T) {
	c := &bump.Check{
		Name:   "aaa",
		Latest: "3",
		Currents: []bump.Current{
			{Version: "1"},
			{Version: "2"},
		},
		Messages: []bump.CheckMessage{
			{Message: "msg1 $NAME/$CURRENT/$LATEST"},
			{Message: "msg2 $NAME/$CURRENT/$LATEST"},
		},
		Links: []bump.CheckLink{
			{Title: "title 1 $NAME/$CURRENT/$LATEST", URL: "https://1/$NAME/$CURRENT/$LATEST"},
			{Title: "title 2 $NAME/$CURRENT/$LATEST", URL: "https://2/$NAME/$CURRENT/$LATEST"},
		},
	}

	tf := githubaction.CheckTemplateReplaceFn(c)

	testCases := []struct {
		template string
		expected string
	}{
		{`Update {{.Name}} from {{join .Current ", "}} to {{.Latest}}`, `Update aaa from 1, 2 to 3`},
		{
			`` +
				`{{range .Messages}}{{.}}{{"\n\n"}}{{end}}` +
				`{{range .Links}}{{.Title}} {{.URL}}{{"\n"}}{{end}}`,
			"" +
				"msg1 aaa/1/3\n\n" +
				"msg2 aaa/1/3\n\n" +
				"title 1 aaa/1/3 https://1/aaa/1/3\n" +
				"title 2 aaa/1/3 https://2/aaa/1/3\n",
		},
		{
			`` +
				`{{range .Messages}}{{.}}{{"\n\n"}}{{end}}` +
				`{{range .Links}}[{{.Title}}]({{.URL}})  {{"\n"}}{{end}}`,
			"" +
				"msg1 aaa/1/3\n\n" +
				"msg2 aaa/1/3\n\n" +
				"[title 1 aaa/1/3](https://1/aaa/1/3)  \n" +
				"[title 2 aaa/1/3](https://2/aaa/1/3)  \n",
		},
		{`bump-{{.Name}}-{{.Latest}}`, `bump-aaa-3`},
	}
	for _, tC := range testCases {
		t.Run(tC.template, func(t *testing.T) {
			actual, err := tf(tC.template)
			if err != nil {
				t.Error(err)
			}
			if tC.expected != actual {
				t.Errorf("expected %q, got %q", tC.expected, actual)
			}
		})
	}
}

// TODO: action iface?

type RoundTripFunc func(*http.Request) (*http.Response, error)

func (r RoundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return r(req)
}

func responseClient(fn func(*http.Request) (interface{}, int)) *http.Client {
	return &http.Client{
		Transport: RoundTripFunc(func(req *http.Request) (*http.Response, error) {
			v, code := fn(req)
			var b []byte
			if v != nil {
				var err error
				b, err = json.Marshal(v)
				if err != nil {
					panic(err)
				}
			}
			return &http.Response{
				StatusCode: code,
				Body:       ioutil.NopCloser(bytes.NewReader(b)),
			}, nil
		}),
	}
}

type execArgs struct {
	args []string
	env  []string
}

type testOS struct {
	//actualWrittenFiles []testCaseFile
	actualStdoutBuf *bytes.Buffer
	actualStderrBuf *bytes.Buffer
	actualExec      []execArgs
}

func (t *testOS) Args() []string { panic("not implemented") }
func (t *testOS) Getenv(name string) string {
	envs := map[string]string{
		"GITHUB_TOKEN":      "sometoken",
		"GITHUB_WORKFLOW":   "workflow",
		"GITHUB_ACTION":     "schedule",
		"GITHUB_ACTOR":      "user",
		"GITHUB_REPOSITORY": "owner/repo",
		"GITHUB_EVENT_NAME": "",
		"GITHUB_EVENT_PATH": "",
		"GITHUB_WORKSPACE":  "",
		"GITHUB_SHA":        "asdsad",
		"GITHUB_REF":        "",
		"GITHUB_HEAD_REF":   "",
		"GITHUB_BASE_REF":   "",

		"INPUT_BUMPFILE":             `Bumpfile`,
		"INPUT_FILES":                ``,
		"INPUT_TITLE_TEMPLATE":       `Update {{.Name}} from {{join .Current ", "}} to {{.Latest}}`,
		"INPUT_COMMIT_BODY_TEMPLATE": `{{range .Messages}}{{.}}{{"\n\n"}}{{end}}{{range .Links}}{{.Title}} {{.URL}}{{"\n"}}{{end}}`,
		"INPUT_PR_BODY_TEMPLATE":     `{{range .Messages}}{{.}}{{"\n\n"}}{{end}}{{range .Links}}[{{.Title}}]({{.URL}})  {{"\n"}}{{end}}`,
		"INPUT_BRANCH_TEMPLATE":      `bump-{{.Name}}-{{.Latest}}`,
		"INPUT_USER_NAME":            `bump`,
		"INPUT_USER_EMAIL":           `bump-action@github`,
	}

	return envs[name]
}
func (t *testOS) Stdout() io.Writer                        { return t.actualStdoutBuf }
func (t *testOS) Stderr() io.Writer                        { return t.actualStderrBuf }
func (t *testOS) WriteFile(name string, data []byte) error { panic("not implemented") }
func (t *testOS) ReadFile(name string) ([]byte, error) {
	//panic("not implemented")

	switch name {
	case "Bumpfile":
		return []byte(`
			test
		`), nil
	case "test":
		return []byte(`
		bump: a /a=(.*)/ static:2
		a=1
	`), nil
	}

	return nil, &os.PathError{Op: "open", Path: name, Err: os.ErrNotExist}

}

func (t *testOS) Glob(pattern string) ([]string, error) {
	return []string{"test"}, nil
}

func (t *testOS) Shell(cmd string, env []string) error { panic("not implemented") }

func (t *testOS) Exec(args []string, env []string) error {
	t.actualExec = append(t.actualExec, execArgs{args: args, env: env})
	return nil
}

func TestRun(t *testing.T) {
	to := &testOS{
		// actualWrittenFiles: []testCaseFile{},
		actualStdoutBuf: &bytes.Buffer{},
		actualStderrBuf: &bytes.Buffer{},
		// actualShells:       []testShell{},
	}

	r := githubaction.Command{
		GHClient: &github.Client{
			Version: "test",
			HTTPClient: responseClient(func(req *http.Request) (interface{}, int) {

				log.Printf("req: %#+v\n", req)

				log.Printf("req.URL.String(): %#+v\n", req.URL.String())

				return nil, 200
			}),
		},
		OS: to,
	}

	log.Printf("to.actualStdoutBuf.String(): %#+v\n", to.actualStdoutBuf.String())
	log.Printf("to.actualStderrBuf.String(): %#+v\n", to.actualStderrBuf.String())
	log.Printf("to.actualExec: %#+v\n", to.actualExec)

	if errs := r.Run(); errs != nil {
		for _, err := range errs {
			t.Error(err)
		}
	}
}
