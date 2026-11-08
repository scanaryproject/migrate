package migrate

import (
	nurl "net/url"
	"testing"
)

func TestFilterCustomQuery(t *testing.T) {
	n, err := nurl.Parse("foo://host?a=b&x-custom=foo&c=d&ok=y")
	if err != nil {
		t.Fatal(err)
	}
	nx := FilterCustomQuery(n).Query()
	if nx.Get("x-custom") != "" {
		t.Fatalf("didn't expect x-custom")
	}
	if nx.Get("ok") != "y" {
		t.Fatalf("expected ok=y, got %v", nx.Get("ok"))
	}
}
