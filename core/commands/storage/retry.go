package storage

import (
	"github.com/Workiva/go-datastructures/queue"
	"sort"
	"sync"
	"time"
)

type RetryQueue struct {
	Queue *queue.Queue
}

type HostNode struct {
	sync.Mutex

	Identity string
	Score int64
	RetryTimes int
	FailTimes int
}

func (node *HostNode) IncrementRetry()  {
	node.Lock()
	defer node.Unlock()

	node.RetryTimes++
}

func (node *HostNode) IncrementFail()  {
	node.Lock()
	defer node.Unlock()

	node.FailTimes++
}

type Hosts []*HostNode

func (h Hosts) Len() int {
	return len(h)
}

func (h Hosts) Less(i, j int) bool {
	return h[i].Score > h[j].Score
}

func (h Hosts) Swap(i, j int)      {
	h[i], h[j] = h[j], h[i]
}

func (h *Hosts) Add(node *HostNode) {
	*h = append(*h, node)
}

func (h Hosts) Sort() {
	sort.Sort(h)
}

// init retry queue with 30 initial capacity
func New(capacity int64) *RetryQueue {
	r := &RetryQueue{Queue:queue.New(capacity)}
	return r
}

func (q *RetryQueue) AddAll(h Hosts) error {
	for _, host := range h {
		if err := q.Queue.Put(host); err != nil {
			return err
		}
	}
	return nil
}

func (q *RetryQueue) Peek() (*HostNode, error) {
	node, err := q.Queue.Peek()
	if err != nil {
		return nil, err
	}
	n := node.(*HostNode)
	return n, nil
}

func (q *RetryQueue) Poll() (*HostNode, error) {
	node, err := q.Queue.Poll(1, time.Millisecond)
	if err != nil {
		return nil, err
	}
	n := node[0].(*HostNode)
	return n, nil
}

func (q *RetryQueue) Offer(h *HostNode) error {
	return q.Queue.Put(h)
}

func (q *RetryQueue) Size() int64 {
	return q.Queue.Len()
}

func (q *RetryQueue) Empty() bool {
	return q.Queue.Empty()
}

