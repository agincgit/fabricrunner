package fabricrunner_test

import (
	"context"
	fr "github.com/agincgit/fabricrunner"
	"github.com/agincgit/fabricrunner/provider/openaicompat"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestPersonalModelDelegatesBoundedManagedReasoning(t *testing.T) {
	personalCalls := 0
	personalServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		personalCalls++
		data, _ := io.ReadAll(r.Body)
		text := "one bounded reasoning question"
		if personalCalls == 2 {
			if !strings.Contains(string(data), "normalized managed answer") {
				t.Error("personal model did not receive managed result")
			}
			text = "consumed managed result"
		}
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = io.WriteString(w, `data: {"choices":[{"index":0,"delta":{"content":"`+text+`"}}]}`+"\n\n"+`data: {"choices":[{"index":0,"delta":{},"finish_reason":"stop"}]}`+"\n\ndata: [DONE]\n\n")
	}))
	defer personalServer.Close()
	managedCalls := 0
	managedServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		managedCalls++
		data, _ := io.ReadAll(r.Body)
		if !strings.Contains(string(data), "one bounded reasoning question") {
			t.Error("delegation input missing")
		}
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = io.WriteString(w, "data: {\"choices\":[{\"index\":0,\"delta\":{\"content\":\"normalized managed answer\"}}]}\n\ndata: {\"choices\":[{\"index\":0,\"delta\":{},\"finish_reason\":\"stop\"}]}\n\ndata: [DONE]\n\n")
	}))
	defer managedServer.Close()
	personal, err := openaicompat.New(openaicompat.Config{Name: "personal", BaseURL: personalServer.URL, AllowInsecureHTTP: true})
	if err != nil {
		t.Fatal(err)
	}
	request := fr.ModelRequest{Model: fr.ModelRef{Provider: "personal", Model: "local"}, ToolChoice: fr.ToolChoice{Mode: fr.ToolChoiceNone}, Messages: []fr.Message{{Role: fr.RoleUser, Content: []fr.ContentPart{{Type: fr.ContentText, Classification: fr.ClassPublic, Text: "Propose one bounded question to delegate"}}}}}
	readText := func(stream fr.ModelStream, err error) string {
		t.Helper()
		if err != nil {
			t.Fatal(err)
		}
		events, err := fr.DrainStream(context.Background(), stream)
		if err != nil {
			t.Fatal(err)
		}
		var result strings.Builder
		for _, event := range events {
			if event.Type == fr.ModelEventTextDelta {
				result.WriteString(event.Text)
			}
		}
		return result.String()
	}
	question := readText(personal.Stream(context.Background(), request))
	engine, delegated, _ := engineFixture(t)
	managed, err := openaicompat.New(openaicompat.Config{Name: "test", BaseURL: managedServer.URL, AllowInsecureHTTP: true})
	if err != nil {
		t.Fatal(err)
	}
	engine.Providers["test"] = managed
	engine.Approver = &testApprover{allow: true}
	delegated.Budget.MaxModelCalls = 1
	delegated.Candidates[0].Zone = fr.ZoneManagedCloud
	delegated.Initial.Messages[0].Content[0].Text = question
	projection := runEngine(t, engine, delegated)
	result := projection.Execution[len(projection.Execution)-1].Result
	request.Messages = append(request.Messages, result.Messages[len(result.Messages)-1])
	if got := readText(personal.Stream(context.Background(), request)); got != "consumed managed result" {
		t.Fatal(got)
	}
	if personalCalls != 2 || managedCalls != 1 {
		t.Fatalf("personal=%d managed=%d", personalCalls, managedCalls)
	}
}
