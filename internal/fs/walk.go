package fs

import (
	"context"
	"path"
	"path/filepath"
	"sync"

	"github.com/OpenListTeam/OpenList/v4/internal/conf"
	"github.com/OpenListTeam/OpenList/v4/internal/model"
	"github.com/OpenListTeam/OpenList/v4/internal/op"
)

// WalkFS traverses filesystem fs starting at name up to depth levels.
//
// WalkFS will stop when current depth > `depth`. For each visited node,
// WalkFS calls walkFn. If a visited file system node is a directory and
// walkFn returns path.SkipDir, walkFS will skip traversal of this node.
func WalkFS(ctx context.Context, depth int, name string, info model.Obj, walkFn func(reqPath string, info model.Obj) error) error {
	// This implementation is based on Walk's code in the standard path/path package.
	walkFnErr := walkFn(name, info)
	if walkFnErr != nil {
		if info.IsDir() && walkFnErr == filepath.SkipDir {
			return nil
		}
		return walkFnErr
	}
	if !info.IsDir() || depth == 0 {
		return nil
	}
	meta, _ := op.GetNearestMeta(name)
	// Read directory names.
	objs, err := List(context.WithValue(ctx, conf.MetaKey, meta), name, &ListArgs{})
	if err != nil {
		return walkFnErr
	}
	for _, fileInfo := range objs {
		filename := path.Join(name, fileInfo.GetName())
		if err := WalkFS(ctx, depth-1, filename, fileInfo, walkFn); err != nil {
			if err == filepath.SkipDir {
				break
			}
			return err
		}
	}
	return nil
}

// WalkFSConcurrent is a parallel version of WalkFS for index building, where
// directory listing is network-bound (remote drivers). It uses a worker pool
// over an unbounded cond-queue (deadlock-free, unlike a bounded channel where
// producers are also consumers). Semantics match WalkFS: depth limit, SkipDir,
// and walkFn invoked once per visited node. Order is not preserved (indexing
// does not require it).
func WalkFSConcurrent(ctx context.Context, depth int, name string, info model.Obj, walkFn func(reqPath string, info model.Obj) error, workers int) error {
	if workers < 1 {
		workers = 1
	}
	type task struct {
		path  string
		obj   model.Obj
		depth int
	}
	var (
		mu       sync.Mutex
		cond     = sync.NewCond(&mu)
		queue    []task
		pending  int
		closed   bool
		errOnce  sync.Once
		firstErr error
	)
	push := func(t task) {
		mu.Lock()
		queue = append(queue, t)
		pending++
		cond.Signal()
		mu.Unlock()
	}
	pop := func() (task, bool) {
		mu.Lock()
		for len(queue) == 0 && !closed {
			cond.Wait()
		}
		if len(queue) == 0 {
			mu.Unlock()
			return task{}, false
		}
		t := queue[len(queue)-1]
		queue = queue[:len(queue)-1]
		mu.Unlock()
		return t, true
	}
	done := func() {
		mu.Lock()
		pending--
		if pending == 0 {
			closed = true
			cond.Broadcast()
		}
		mu.Unlock()
	}
	worker := func() {
		for {
			t, ok := pop()
			if !ok {
				return
			}
			if ctx.Err() != nil {
				done()
				continue
			}
			e := walkFn(t.path, t.obj)
			if e != nil {
				if !(t.obj.IsDir() && e == filepath.SkipDir) {
					errOnce.Do(func() { firstErr = e })
				}
				done()
				continue
			}
			if !t.obj.IsDir() || t.depth == 0 {
				done()
				continue
			}
			meta, _ := op.GetNearestMeta(t.path)
			objs, err := List(context.WithValue(ctx, conf.MetaKey, meta), t.path, &ListArgs{})
			if err == nil {
				for _, child := range objs {
					push(task{path: path.Join(t.path, child.GetName()), obj: child, depth: t.depth - 1})
				}
			}
			done()
		}
	}
	var wg sync.WaitGroup
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			worker()
		}()
	}
	push(task{path: name, obj: info, depth: depth})
	wg.Wait()
	return firstErr
}
