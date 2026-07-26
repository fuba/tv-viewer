package programguide

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestClientServicesAndSearch(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/services":
			_, _ = w.Write([]byte(`[{"id":3273601024,"serviceId":1024,"name":"NHK","channelType":"GR","channelNumber":"27"}]`))
		case "/search":
			if r.URL.Query().Get("startFrom") == "" || r.URL.Query().Get("startTo") == "" {
				t.Errorf("missing time range: %s", r.URL.RawQuery)
			}
			if r.URL.Query().Get("channelType") != "GR" {
				t.Errorf("unexpected channel type: %s", r.URL.Query().Get("channelType"))
			}
			_, _ = w.Write([]byte(`[{"id":1,"serviceId":1024,"startAt":1700000000000,"duration":3600000,"name":"News"}]`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	client := NewClient(server.URL)
	services, err := client.Services()
	if err != nil || len(services) != 1 || services[0].ServiceID != 1024 {
		t.Fatalf("unexpected services: %#v, %v", services, err)
	}
	programs, err := client.Search(time.UnixMilli(1700000000000), time.UnixMilli(1700003600000), "GR")
	if err != nil || len(programs) != 1 || programs[0].Name != "News" {
		t.Fatalf("unexpected programs: %#v, %v", programs, err)
	}
}
