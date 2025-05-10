package commit

//
// support for 3PC tester.
//
// we will use the original config.go to test your code for grading.
// so, while you can modify this code to help you debug, please
// test with the original before submitting.
//

import (
	"3PhaseCommit/labrpc"
	"runtime"
	"sync"
	"testing"

	crand "crypto/rand"
	"encoding/base64"
	"fmt"
	"time"
)



func randstring(n int) string {
	b := make([]byte, 2*n)
	crand.Read(b)
	s := base64.URLEncoding.EncodeToString(b)
	return s[0:n]
}

type config struct {
	mu            sync.Mutex
	t             *testing.T
	net           *labrpc.Network
	n             int
	keyMap        map[string]int // which keys are assigned to which servers
	coordinator   *Coordinator   // protected by `mu`
	servers       []*Server      // protected by `mu`
	transactions  []ResponseMsg  // protected by `mu`
	connected     []bool         // whether each server is on the net; protected by `mu`
	endnames      []string       // the port file names the coordinator sends to
	doOnPreCommit func() bool    // function to run on next PreCommit
	doOnCommit    func() bool    // function to run on next Commit
	start         time.Time      // time at which make_config() was called
	// begin()/end() statistics
	t0        time.Time // time at which test_test.go called cfg.begin()
	rpcs0     int       // rpcTotal() at start of test
	bytes0    int64
	maxIndex  int // protected by `mu`
	maxIndex0 int
	stopCh    chan struct{}
}

var ncpu_once sync.Once

func make_config(t *testing.T, keys [][]string, unreliable bool, snapshot bool) *config {
	ncpu_once.Do(func() {
		if runtime.NumCPU() < 2 {
			fmt.Printf("warning: only one CPU, which may conceal locking bugs\n")
		}
	})

	runtime.GOMAXPROCS(4)
	cfg := &config{}
	cfg.t = t
	cfg.net = labrpc.MakeNetwork()
	cfg.n = len(keys)
	cfg.keyMap = make(map[string]int)
	cfg.servers = make([]*Server, cfg.n)
	cfg.connected = make([]bool, cfg.n)
	cfg.endnames = make([]string, cfg.n)
	cfg.start = time.Now()

	cfg.setunreliable(unreliable)

	cfg.net.LongDelays(false)

	cfg.net.RegisterCallback(cfg.netCallback)

	for i, keyList := range keys {
		for _, key := range keyList {
			cfg.keyMap[key] = i
		}
	}

	for i := 0; i < cfg.n; i++ {
		cfg.startServer(i, keys[i])
	}
	cfg.startCoordinator()
	// connect everyone
	for i := 0; i < cfg.n; i++ {
		cfg.connect(i)
	}

	return cfg
}

func (cfg *config) netCallback(method string, endname interface{}) {
	cfg.mu.Lock()
	defer cfg.mu.Unlock()

	if method == "Server.PreCommit" && cfg.doOnPreCommit != nil {
		if cfg.doOnPreCommit() {
			cfg.doOnPreCommit = nil
		}
	}

	if method == "Server.Commit" && cfg.doOnCommit != nil {
		if cfg.doOnCommit() {
			cfg.doOnCommit = nil
		}
	}
}

func (cfg *config) doNextPreCommit(f func() bool) {
	cfg.mu.Lock()
	defer cfg.mu.Unlock()

	cfg.doOnPreCommit = f
}

func (cfg *config) doNextCommit(f func() bool) {
	cfg.mu.Lock()
	defer cfg.mu.Unlock()

	cfg.doOnCommit = f
}

func (cfg *config) restartCoordinatorLocked() {
	cfg.crashCoordinatorLocked()
	cfg.coordinator = cfg.newCoordinator()
	cfg.connectAll()
}

func (cfg *config) apply(m ResponseMsg) {
	cfg.mu.Lock()
	defer cfg.mu.Unlock()

	for _, trans := range cfg.transactions {
		if trans.tid == m.tid {
			cfg.t.Fatalf("Got repeated client message for transaction %d", m.tid)
		}
	}

	cfg.transactions = append(cfg.transactions, m)
}

// applier reads message from response channel
func (cfg *config) applier(respChan chan ResponseMsg, stopCh <-chan struct{}) {
	for {
		select {
		case <-stopCh:
			{
				// When applier is stopped, it still continues to empty `applyCh`.
				stopCh = nil
			}
		case m, ok := <-respChan:
			{
				if !ok {
					return
				}
				if stopCh != nil {
					cfg.apply(m)
				}
			}
		}
	}
}

// start a new Server
func (cfg *config) startServer(i int, keys []string) {
	sv := MakeServer(keys)

	cfg.mu.Lock()
	cfg.servers[i] = sv
	cfg.mu.Unlock()

	svc := labrpc.MakeService(sv)
	srv := labrpc.MakeServer()
	srv.AddService(svc)
	cfg.net.AddServer(i, srv)
}

func (cfg *config) crashCoordinator() {
	cfg.mu.Lock()
	defer cfg.mu.Unlock()

	cfg.crashCoordinatorLocked()
}

// shut down a Raft server but save its persistent state.
func (cfg *config) crashCoordinatorLocked() {
	if cfg.coordinator == nil {
		return
	}

	// disconnect outgoing ClientEnds
	for _, endname := range cfg.endnames {
		cfg.net.Enable(endname, false)
	}

	close(cfg.stopCh)
	cfg.coordinator.Kill()

	cfg.coordinator = nil
}

func (cfg *config) newCoordinator() *Coordinator {
	// a fresh set of outgoing ClientEnd names.
	// so that old crashed instance's ClientEnds can't send.
	cfg.endnames = make([]string, cfg.n)
	for i := range cfg.n {
		cfg.endnames[i] = randstring(20)
	}

	// a fresh set of ClientEnds.
	ends := make([]*labrpc.ClientEnd, cfg.n)
	for i := range cfg.n {
		ends[i] = cfg.net.MakeEnd(cfg.endnames[i])
		cfg.net.Connect(cfg.endnames[i], i)
	}

	respChan := make(chan ResponseMsg)
	cfg.stopCh = make(chan struct{})
	go cfg.applier(respChan, cfg.stopCh)

	return MakeCoordinator(ends, respChan)
}

// start or re-start the Coordinator.
// if one already exists, "kill" it first.
// allocate new outgoing port file names to
// isolate previous instance of this server.
// since we cannot really kill it.
func (cfg *config) startCoordinator() {
	cfg.crashCoordinator()

	co := cfg.newCoordinator()

	cfg.mu.Lock()
	cfg.coordinator = co
	cfg.mu.Unlock()
}

func (cfg *config) checkTimeout() {
	// enforce a two minute real-time limit on each test
	if !cfg.t.Failed() && time.Since(cfg.start) > 120*time.Second {
		cfg.t.Fatal("test took longer than 120 seconds")
	}
}

func (cfg *config) cleanup() {
	/*
		for i := 0; i < len(cfg.rafts); i++ {
			if cfg.rafts[i] != nil {
				cfg.rafts[i].Kill()
			}
		}
	*/
	cfg.coordinator.Kill()
	cfg.net.Cleanup()
	cfg.checkTimeout()
}

// attach server i to the net.
func (cfg *config) connect(i int) {
	// fmt.Printf("connect(%d)\n", i)

	cfg.connected[i] = true

	cfg.net.Enable(cfg.endnames[i], true)
}

func (cfg *config) connectAll() {
	for i := range cfg.servers {
		cfg.connect(i)
	}
}

// detach server i from the net.
func (cfg *config) disconnect(i int) {
	// fmt.Printf("disconnect(%d)\n", i)

	cfg.connected[i] = false

	cfg.net.Enable(cfg.endnames[i], false)
}

func (cfg *config) rpcCount(server int) int {
	return cfg.net.GetCount(server)
}

func (cfg *config) rpcTotal() int {
	return cfg.net.GetTotalCount()
}

func (cfg *config) setunreliable(unrel bool) {
	cfg.net.Reliable(!unrel)
}

func (cfg *config) bytesTotal() int64 {
	return cfg.net.GetTotalBytes()
}

func (cfg *config) setlongreordering(longrel bool) {
	cfg.net.LongReordering(longrel)
}

func (cfg *config) sendGet(tid int, key string) {
	cfg.mu.Lock()
	defer cfg.mu.Unlock()

	cfg.servers[cfg.keyMap[key]].Get(tid, key)
}

func (cfg *config) sendSet(tid int, key string, value interface{}) {
	cfg.mu.Lock()
	defer cfg.mu.Unlock()

	cfg.servers[cfg.keyMap[key]].Set(tid, key, value)
}

func (cfg *config) finishTransaction(tid int) {
	cfg.mu.Lock()
	defer cfg.mu.Unlock()

	go cfg.coordinator.FinishTransaction(tid)
}

func (cfg *config) waitTransaction(tid int) ResponseMsg {
	for {
		cfg.mu.Lock()
		for _, trans := range cfg.transactions {
			if trans.tid == tid {
				defer cfg.mu.Unlock()
				return trans
			}
		}
		cfg.mu.Unlock()
		time.Sleep(10 * time.Millisecond)
	}
}

func (cfg *config) assertNoTransaction(tid int) {
	cfg.mu.Lock()
	defer cfg.mu.Unlock()
	for _, trans := range cfg.transactions {
		if trans.tid == tid {
			cfg.t.Fatalf("Expected there to be no transaction %d", tid)
		}
	}
}

func (cfg *config) assertTransaction(tid int, committed bool, readValues map[string]interface{}) ResponseMsg {
	resp := cfg.waitTransaction(tid)
	if resp.committed != committed {
		if committed {
			cfg.t.Fatalf("Transaction %d expected to be committed but wasn't", tid)
		} else {
			cfg.t.Fatalf("Transaction %d expected to be aborted but wasn't", tid)
		}
	}
	// Ignore readValues check if it's nil
	if readValues == nil {
		return resp
	}
	if len(resp.readValues) != len(readValues) {
		cfg.t.Fatalf("Transaction %d returned incorrect number of keys", tid)
	}
	for k, v1 := range readValues {
		v2, pres := resp.readValues[k]
		if !pres {
			cfg.t.Fatalf("Transaction %d failed to return key %s", tid, k)
		}
		if v1 != v2 {
			cfg.t.Fatalf("Transaction %d returned incorrect value %v for key %s", tid, v2, k)
		}
	}
	return resp
}

// start a Test.
// print the Test message.
// e.g. cfg.begin("Test (2B): RPC counts aren't too high")
func (cfg *config) begin(description string) {
	fmt.Printf("%s ...\n", description)
	cfg.t0 = time.Now()
	cfg.rpcs0 = cfg.rpcTotal()
	cfg.bytes0 = cfg.bytesTotal()
	cfg.maxIndex0 = cfg.maxIndex
}

// end a Test -- the fact that we got here means there
// was no failure.
// print the Passed message,
// and some performance numbers.
func (cfg *config) end() {
	cfg.checkTimeout()
	if cfg.t.Failed() == false {
		cfg.mu.Lock()
		t := time.Since(cfg.t0).Seconds()       // real time
		npeers := cfg.n                         // number of Raft peers
		nrpc := cfg.rpcTotal() - cfg.rpcs0      // number of RPC sends
		nbytes := cfg.bytesTotal() - cfg.bytes0 // number of bytes
		ncmds := cfg.maxIndex - cfg.maxIndex0   // number of Raft agreements reported
		cfg.mu.Unlock()

		fmt.Printf("  ... Passed --")
		fmt.Printf("  %4.1f  %d %4d %7d %4d\n", t, npeers, nrpc, nbytes, ncmds)
	}
}
