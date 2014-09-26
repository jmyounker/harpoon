package main

import "sync"

type workQueue struct {
	todo   []func()
	lock   sync.Mutex
	notify chan struct{}
	quitc  chan struct{}
}

func newWorkQueue() *workQueue {
	wq := &workQueue{
		todo:   []func(){},
		lock:   sync.Mutex{},
		notify: make(chan struct{}),
		quitc:  make(chan struct{}),
	}
	go wq.run()
	return wq
}

func (wq *workQueue) run() {
	for {
		wq.lock.Lock()
		select {
		case <-wq.quitc:
			wq.lock.Unlock()
			return
		default:
		}
		if len(wq.todo) == 0 {
			wq.lock.Unlock()
			select {
			case <-wq.notify:
			case <-wq.quitc:
				return
			}
			wq.lock.Lock()
		}
		f := wq.todo[0]
		wq.todo = wq.todo[1:]
		wq.lock.Unlock()
		f()
	}
}

func (wq *workQueue) push(f func()) {
	wq.lock.Lock()
	defer wq.lock.Unlock()

	wq.todo = append(wq.todo, f)
	select {
	case wq.notify <- struct{}{}:
	default:
	}
}

func (wq *workQueue) quit() {
	wq.quitc <- struct{}{}
}
