package enola

import (
	"context"
	_ "embed"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/benji-bou/diwo"
)

const RequestTimeout time.Duration = time.Second * 20

type Website struct {
	ErrorType         string      `json:"errorType"`
	ErrorMessage      interface{} `json:"errorMsg"`
	URL               string      `json:"url"`
	UrlMain           string      `json:"urlMain"`
	UsernameClaimed   string      `json:"username_claimed"`
	UsernameUnclaimed string      `json:"username_unclaimed"`
}

type Enola struct {
	Data map[string]Website
	Site string
	Ctx  context.Context
}

type Result struct {
	Name  string `json:"name"`
	URL   string `json:"url"`
	Found bool   `json:"found"`
}

//go:embed data.json
var d []byte

func New(ctx context.Context) (Enola, error) {
	var data map[string]Website
	err := json.Unmarshal(d, &data)
	if err != nil {
		return Enola{}, fmt.Errorf("error: %v", ErrDataFileIsNotAValidJson)
	}

	return Enola{
		Data: data,
		Ctx:  ctx,
	}, nil
}

func (s *Enola) SetSite(site string) *Enola {
	s.Site = site
	return s
}

func (s *Enola) ListCount() int           { return len(s.Data) }
func (s *Enola) List() map[string]Website { return s.Data }

func (s *Enola) Check(username string) (<-chan Result, error) {
	data := map[string]Website{}
	if s.Site != "" {
		for k, v := range s.Data {
			if strings.Contains(strings.ToLower(k), strings.Trim(strings.ToLower(s.Site), " ")) {
				data[k] = v
			}
		}

		// if site is not found in the list
		if len(data) == 0 {
			return nil, fmt.Errorf("error: %v", ErrSiteNotFound)
		}
	}

	if len(data) == 0 {
		data = s.Data
	}
	dataC := diwo.FromMap(data)
	return diwo.MergedPool[Result](20, func(outputC chan<- Result) {
		for data := range dataC {
			outputC <- Check(data.Key, data.Value, username)
		}
	}), nil
}

func Check(key string, value Website, username string) Result {
	url := strings.ReplaceAll(value.URL, "{}", username)

	res := Result{
		Name:  key,
		URL:   url,
		Found: false,
	}

	client := http.DefaultClient
	client.Timeout = RequestTimeout
	if value.ErrorType == "status_code" {
		resp, err := client.Get(url)
		if err != nil {
			return res
		}
		resp.Body.Close()

		if resp.StatusCode == http.StatusOK {
			res.Found = true

			return res
		}
		return res

	}

	if value.ErrorType == "message" {
		resp, err := client.Get(url)
		if err != nil {
			return res
		}
		defer resp.Body.Close()

		bodyBytes, err := io.ReadAll(resp.Body)
		if err != nil {
			return res
		}

		valueString, ok := value.ErrorMessage.(string)
		if !ok {
			return res
		}

		if !strings.Contains(string(bodyBytes), valueString) {
			res.Found = true
			return res
		}
	}
	return res
}
