package pluginkit

import (
	"reflect"
	"testing"
)

func TestListKinds(t *testing.T) {
	resetRegistry()
	t.Cleanup(resetRegistry)

	if kinds := ListKinds(); len(kinds) != 0 {
		t.Fatalf("ListKinds()=%v want empty", kinds)
	}

	for _, kind := range []string{"beta", "alpha"} {
		if err := register(kind, func() (*describePlugin, error) {
			return &describePlugin{}, nil
		}); err != nil {
			t.Fatal(err)
		}
	}
	if kinds := ListKinds(); !reflect.DeepEqual(kinds, []string{"alpha", "beta"}) {
		t.Fatalf("ListKinds()=%v", kinds)
	}
}

func TestCompatibleKinds(t *testing.T) {
	resetRegistry()
	t.Cleanup(resetRegistry)

	llmType := reflect.TypeOf((*LLM)(nil)).Elem()
	toolType := reflect.TypeOf((*Tool)(nil)).Elem()

	if err := register("openai", func(openaiCfg) (*openaiStub, error) {
		return &openaiStub{}, nil
	}); err != nil {
		t.Fatal(err)
	}
	if err := register("read-file", func(readFileCfg) (*readFileStub, error) {
		return &readFileStub{}, nil
	}); err != nil {
		t.Fatal(err)
	}
	if err := register("shell", func() (*shellStub, error) {
		return &shellStub{}, nil
	}); err != nil {
		t.Fatal(err)
	}

	got := CompatibleKinds(llmType)
	if len(got) != 1 || got[0] != "openai" {
		t.Fatalf("CompatibleKinds(LLM)=%v want [openai]", got)
	}

	got = CompatibleKinds(toolType)
	if len(got) != 2 || got[0] != "read-file" || got[1] != "shell" {
		t.Fatalf("CompatibleKinds(Tool)=%v want [read-file shell]", got)
	}
}

func TestCompatibleKindsPrefersExactReturnType(t *testing.T) {
	resetRegistry()
	t.Cleanup(resetRegistry)

	toolType := reflect.TypeOf((*Tool)(nil)).Elem()
	stepType := reflect.TypeOf((*Step)(nil)).Elem()

	if err := register("read-file", func(readFileCfg) (Tool, error) {
		return &readFileStub{}, nil
	}); err != nil {
		t.Fatal(err)
	}
	if err := register("shell", func() (Tool, error) {
		return &shellStub{}, nil
	}); err != nil {
		t.Fatal(err)
	}
	if err := register("openai", func(openaiCfg) (LLM, error) {
		return &openaiStub{}, nil
	}); err != nil {
		t.Fatal(err)
	}
	if err := register("http-step", func() (Step, error) {
		return stepStub{}, nil
	}); err != nil {
		t.Fatal(err)
	}

	got := CompatibleKinds(toolType)
	want := []string{"read-file", "shell", "http-step", "openai"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("CompatibleKinds(Tool)=%v want %v", got, want)
	}

	got = CompatibleKinds(stepType)
	wantStep := []string{"http-step", "openai", "read-file", "shell"}
	if !reflect.DeepEqual(got, wantStep) {
		t.Fatalf("CompatibleKinds(Step)=%v want %v", got, wantStep)
	}
}

type Step interface {
	Run() string
}

type openaiStub struct{}

func (o *openaiStub) Model() string { return "" }

type readFileStub struct{}

func (r *readFileStub) Name() string { return "" }

type shellStub struct{}

func (s *shellStub) Name() string { return "" }

type stepStub struct{}

func (s stepStub) Run() string { return "" }

type openaiCfg struct {
	Model string `json:"model"`
}

type readFileCfg struct {
	Root string `json:"root"`
}
