package master

import (
	"os"
	"os/signal"
	"sync"
	"syscall"
)

type worker struct {
	waitGroup sync.WaitGroup
	shutdown  chan struct{}
	lns       []*Listener
	events    []func()
}

func newWorker(lns []*Listener) *worker {
	w := &worker{
		shutdown: make(chan struct{}),
		lns:      lns,
	}
	for _, ln := range w.lns {
		ln.worker = w
	}
	if len(w.lns) > 0 {
		w.waitGroup.Add(len(w.lns))
	}
	return w
}

func (w *worker) run() {
	var (
		once    sync.Once
		done    = make(chan struct{})
		sigHub  = make(chan os.Signal)
		sigExit = make(chan os.Signal)
	)
	signal.Notify(sigHub, syscall.SIGHUP)
	signal.Notify(sigExit, syscall.SIGINT)
	signal.Notify(sigExit, syscall.SIGTERM)
	defer signal.Stop(sigHub)
	defer signal.Stop(sigExit)

	//优雅退出事件处理
	go func() {
		select {
		case <-done:
		case <-sigHub:
			//关闭ln并等待conn关闭
			w.closeAllListeners()
			w.waitListenerAndConnectionClose()
			once.Do(func() { close(done) })
		}
	}()
	//强制退出事件处理
	go func() {
		select {
		case <-done:
		case <-sigExit:
			w.closeAllListeners()
			once.Do(func() { close(done) })
		}
	}()

	<-done
	//广播退出事件
	for _, f := range w.events {
		f()
	}
	close(w.shutdown)
}

func (w *worker) waitListenerAndConnectionClose() {
	w.waitGroup.Wait()
	for _, v := range w.lns {
		v.wg.Wait()
	}
}

func (w *worker) waitQuit() {
	w.waitListenerAndConnectionClose()
	<-w.shutdown
}

func (w *worker) registerExitEvent(event func()) {
	w.events = append(w.events, event)
}

func (w *worker) listeners() []*Listener {
	return w.lns
}

func (w *worker) closeAllListeners() {
	for _, ln := range w.lns {
		_ = ln.Close()
	}
}
