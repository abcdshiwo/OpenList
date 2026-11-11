package refreshguard

import (
	"sync"
	"testing"
	"time"

	"github.com/OpenListTeam/OpenList/v4/internal/model"
)

func TestLimiterBurstAndRefill(t *testing.T) {
	l := NewLimiter()
	now := time.Now()
	rate := 1.0 / 10.0 // one token per 10 seconds
	for i := 0; i < 3; i++ {
		if ok, _ := l.Allow("u1:/a", rate, 3, now); !ok {
			t.Fatalf("burst token %d should be allowed", i)
		}
	}
	if ok, wait := l.Allow("u1:/a", rate, 3, now); ok {
		t.Fatal("4th call within burst should be denied")
	} else if wait < 9*time.Second || wait > 11*time.Second {
		t.Fatalf("expected wait about 10s, got %v", wait)
	}
	if ok, _ := l.Allow("u1:/a", rate, 3, now.Add(10*time.Second)); !ok {
		t.Fatal("token should be refilled after 10s")
	}
}

func TestLimiterSeparateKeys(t *testing.T) {
	l := NewLimiter()
	now := time.Now()
	l.Allow("u1:/a", 0.1, 1, now)
	if ok, _ := l.Allow("u1:/b", 0.1, 1, now); !ok {
		t.Fatal("different key should have its own bucket")
	}
}

func TestGroupDeduplicates(t *testing.T) {
	g := NewGroup()
	var calls int
	var mu sync.Mutex
	fn := func() ([]model.Obj, error) {
		mu.Lock()
		calls++
		mu.Unlock()
		time.Sleep(20 * time.Millisecond)
		return []model.Obj{&model.Object{Name: "x"}}, nil
	}
	var wg sync.WaitGroup
	for i := 0; i < 5; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			objs, err := g.Do("/same", fn)
			if err != nil || len(objs) != 1 {
				t.Errorf("unexpected result: %v %v", objs, err)
			}
		}()
	}
	wg.Wait()
	if calls != 1 {
		t.Fatalf("expected 1 upstream call, got %d", calls)
	}
}

func TestCounter(t *testing.T) {
	c := NewCounter()
	if !c.TryEnter(2) || !c.TryEnter(2) {
		t.Fatal("first two should enter")
	}
	if c.TryEnter(2) {
		t.Fatal("third should be rejected")
	}
	c.Leave()
	if !c.TryEnter(2) {
		t.Fatal("should enter after leave")
	}
}
