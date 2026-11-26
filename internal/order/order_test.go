package order

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/dorako321/go-kojo/internal/engine"
)

// staff answers for each role from prepared replies and records who was
// asked what, with which model.
type staff struct {
	reception []string
	design    []string
	review    []string
	asked     []string // "role:model"
	prompts   map[string][]string
}

func (s *staff) Call(_ context.Context, req engine.Request) (engine.Response, error) {
	role, queue := "design", &s.design
	switch {
	case strings.Contains(req.Schema, `"brief"`):
		role, queue = "reception", &s.reception
	case strings.Contains(req.Schema, `"predictable"`):
		role, queue = "review", &s.review
	}
	s.asked = append(s.asked, role+":"+req.Model)
	if s.prompts == nil {
		s.prompts = map[string][]string{}
	}
	s.prompts[role] = append(s.prompts[role], req.Prompt)
	out := (*queue)[0]
	*queue = (*queue)[1:]
	return engine.Response{Output: json.RawMessage(out), CostUSD: 0.01}, nil
}

// proposal is a designer's reply for a program that upper-cases its input,
// expecting the given stdout for the input "hi\n".
func proposal(name, want string) string {
	w := wireReply{Action: "propose", Message: "作ります", Spec: &wireSpec{
		Name: name, Summary: name + " prints its input in upper case.", Kind: "convert", Usage: name,
		Behavior: []string{"B1: Copy stdin to stdout in upper case."},
		Cases: []wireCase{
			{Name: "hi", Stdin: "hi\n", Exit: 0, Stdout: wireExpect{"exact", want}, Stderr: wireExpect{"any", ""}},
			{Name: "empty", Exit: 0, Stdout: wireExpect{"exact", ""}, Stderr: wireExpect{"any", ""}},
		},
	}}
	b, _ := json.Marshal(w)
	return string(b)
}

const handoff = `{"action":"handoff","message":"","options":[],"brief":"Upper-case stdin."}`

var team = Models{Reception: "fast", Design: "work", Review: "strong"}

func TestDeskAsksBeforeDesigning(t *testing.T) {
	s := &staff{reception: []string{`{"action":"ask","message":"どちら？","options":["A","B"],"brief":""}`}}
	rep, err := Desk(context.Background(), s, team, []Turn{{"customer", "何か"}}, t.TempDir(), nil)
	if err != nil {
		t.Fatal(err)
	}
	if rep.Action != "ask" || len(rep.Options) != 2 || strings.Join(s.asked, " ") != "reception:fast" {
		t.Errorf("action %q, options %q, asked %q", rep.Action, rep.Options, s.asked)
	}
}

func TestDeskReviewAgrees(t *testing.T) {
	s := &staff{
		reception: []string{handoff},
		design:    []string{proposal("kojotestupper", "HI\n")},
		review:    []string{`{"cases":[{"name":"hi","predictable":true,"exit":0,"stdout":"HI\n","note":""},{"name":"empty","predictable":true,"exit":0,"stdout":"","note":""}]}`},
	}
	var steps []string
	rep, err := Desk(context.Background(), s, team, []Turn{{"customer", "大文字にしたい"}}, t.TempDir(), func(st Step) {
		steps = append(steps, st.Role+"/"+st.State)
	})
	if err != nil {
		t.Fatal(err)
	}
	if rep.Action != "propose" || rep.Reviewed != 2 || len(rep.Disputed) != 0 || rep.Revised {
		t.Errorf("reply %+v", rep)
	}
	if got := strings.Join(s.asked, " "); got != "reception:fast design:work review:strong" {
		t.Errorf("each role should use its own model, got %q", got)
	}
	// The reviewer must not be shown what the designer expects.
	if p := s.prompts["review"][0]; strings.Contains(p, `"HI`) || strings.Contains(p, "exact") {
		t.Errorf("the review prompt leaks the expected values:\n%s", p)
	}
	if got := strings.Join(steps, " "); got != "reception/start reception/done design/start design/done review/start review/done" {
		t.Errorf("steps %q", got)
	}
}

// When the two disagree, the designer hears the reviewer's result and gets
// one chance to put the spec right.
func TestDeskReviewDisagrees(t *testing.T) {
	s := &staff{
		reception: []string{handoff},
		design:    []string{proposal("kojotestupper", "Hi\n"), proposal("kojotestupper", "HI\n")}, // first expectation is wrong
		review:    []string{`{"cases":[{"name":"hi","predictable":true,"exit":0,"stdout":"HI\n","note":""},{"name":"empty","predictable":false,"exit":0,"stdout":"","note":""}]}`},
	}
	rep, err := Desk(context.Background(), s, team, []Turn{{"customer", "大文字にしたい"}}, t.TempDir(), nil)
	if err != nil {
		t.Fatal(err)
	}
	if rep.Reviewed != 1 || strings.Join(rep.Disputed, ",") != "hi" || !rep.Revised {
		t.Fatalf("reviewed %d, disputed %q, revised %v", rep.Reviewed, rep.Disputed, rep.Revised)
	}
	if got := *rep.Spec.Cases[0].Stdout.Exact; got != "HI\n" {
		t.Errorf("the corrected expectation was not adopted: %q", got)
	}
	second := s.prompts["design"][1]
	for _, want := range []string{"review:", `case "hi"`, `the reviewer derives: exit 0, stdout "HI\n"`, `want exactly "Hi\n"`} {
		if !strings.Contains(second, want) {
			t.Errorf("the designer was not told %q:\n%s", want, second)
		}
	}
}

// A failed build goes straight to the designer; the customer is not asked.
func TestDeskSkipsReceptionAfterFactoryNote(t *testing.T) {
	s := &staff{design: []string{proposal("kojotestupper", "HI\n")}}
	turns := []Turn{{"customer", "大文字にしたい"}, {"factory", "the build ended with status failed"}}
	if _, err := Desk(context.Background(), s, Models{Reception: "fast", Design: "work"}, turns, t.TempDir(), nil); err != nil {
		t.Fatal(err)
	}
	if got := strings.Join(s.asked, " "); got != "design:work" {
		t.Errorf("asked %q", got)
	}
}

// A spec the line cannot take is sent back to the designer with the reason.
func TestDeskSendsBackUnbuildableSpec(t *testing.T) {
	s := &staff{reception: []string{handoff}, design: []string{proposal("ls", "HI\n"), proposal("kojotestupper", "HI\n")}}
	rep, err := Desk(context.Background(), s, Models{Reception: "fast", Design: "work"}, []Turn{{"customer", "大文字にしたい"}}, t.TempDir(), nil)
	if err != nil {
		t.Fatal(err)
	}
	if rep.Spec.Name != "kojotestupper" {
		t.Errorf("name %q", rep.Spec.Name)
	}
	if p := s.prompts["design"][1]; !strings.Contains(p, `the name "ls" is already a command`) {
		t.Errorf("the designer was not told why:\n%s", p)
	}
}
