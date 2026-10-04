package terminal

import "testing"

func TestManagerRejectsBadDefs(t *testing.T) {
	dir := t.TempDir()
	mk := func(id string) Def { return Def{ID: id, Config: Config{Shell: "/bin/sh", Dir: dir}} }
	if _, err := NewManager([]Def{mk("a"), mk("a")}); err == nil {
		t.Error("duplicate ids accepted")
	}
	if _, err := NewManager([]Def{mk("")}); err == nil {
		t.Error("empty id accepted")
	}
}

func TestManagerListOrderAndExit(t *testing.T) {
	dir := t.TempDir()
	m, err := NewManager([]Def{
		{ID: "b", Config: Config{Name: "B", Shell: "/bin/sh", Dir: dir}},
		{ID: "a", Config: Config{Name: "A", Shell: "/bin/sh", Dir: dir}},
	})
	if err != nil {
		t.Fatal(err)
	}
	defer m.Close()
	l := m.List()
	if len(l) != 2 || l[0].ID != "b" || l[1].ID != "a" || l[0].Dir != dir {
		t.Fatalf("list: %+v", l)
	}
	s, _ := m.Get("a")
	s.Write([]byte("exit 4\n"))
	<-s.Done()
	if l := m.List(); !l[1].Exited || l[1].ExitCode != 4 || l[0].Exited {
		t.Fatalf("after exit: %+v", l)
	}
	if err := m.Restart("b"); err != ErrRunning {
		t.Fatalf("restart running: %v", err)
	}
}
