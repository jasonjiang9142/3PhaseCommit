# Assignment 4: 3PC

**Due: Thu, May 1**

## Three-Phase Commit

Three-Phase Commit (3PC) is an extended version of the Two-Phase Commit (2PC) protocol that we've studied in this course. For this assignment, you'll be implementing this protocol to ensure atomic transactions (i.e. sequences of get/set requests) on a distributed key-value store. For our purposes, the main advantage of 3PC over 2PC is that the transaction coordinator doesn't need to keep any persistent state. This means that in theory, any one of the servers could take over as coordinator should the initial one fail. For this assignment, this means that some of the tests will crash and restart your coordinator and you'll be tested on its ability to recover from this failure. 

## The Protocol

Here is the 3PC protocol as you should implement it for this assignment:

1. A client submits Get and Set operations to individual servers. The servers log these operations, but don't execute them yet.

2. The client sends a FinishTransaction message to the transaction coordinator.

3. The coordinator sends Prepare messages to each server. If a server receives Prepare for a transaction that it does not have any operations logged for, it replies that the transaction isn't relevant to it. Future PreCommit and Commit messages for this transaction should only be sent to the relevant servers. Each server attempts to obtain a lock for each key that the transaction uses. If this succeeds, the server votes Yes. Otherwise, the server releases any locks it took and votes No. If any servers vote No or timeout, the coordinator Aborts the transaction.


4. Otherwise, the coordinator sends PreCommit messages to each relevant server. Each server just needs to acknowledge PreCommit. If any requests timeout, it Aborts this transaction.

5. Otherwise, the coordinator sends Commit messages to each relevant server. Each server performs its operations for the transaction and replies with the values of any Get operations. If any requests timeout, the coordinator **resends** them until the server responds.
6. Then the coordinator replies to the client that the transaction has been committed and sends the client the values of all Get operations in that transaction.

### A Note on Concurrency

To help avoid concurrency issues, we will use shared mutexes to protect individual keys. This means that at any given time, either one transaction can lock a key for writing, or several transactions can lock the key for reading. Go's implementation of this is `sync.RWMutex`. To lock/unlock for writing, use `Lock()` and `Unlock()`. To lock/unlock for reading, use `RLock()` and `RUnlock()`.

### Aborting

If at any point during the protocol the coordinator decides to Abort the transaction, it should send Abort messages to all servers and respond to the client to inform it that the transaction has been aborted. If any requests timeout, the coordinator should resend them until the server responds.

### Coordinator Recovery

The coordinator can crash at any time. When it restarts, it should send Query messages to each server to get an understanding of the state of the system. Servers should respond with the state of each transaction that it knows about. If any requests timeout, the coordinator **resends** them until the server responds. The coordinator **cannot** finish recovering until it has successfully Queried each server. Then the coordinator makes the following checks to resume the system:

- For each transaction that any server has already voted no for or aborted, the coordinator Aborts that transaction.
- For each remaining transaction that some, but not all, servers have already committed, the coordinator resumes the protocol for that transaction from sending out Commit messages.
- For each remaining transaction that any server has pre-committed, the coordinator resumes the protocol for that transaction from sending out PreCommit messages.
- For each remaining transaction that any server has voted yes for, the coordinator resumes the protocol for that transaction from sending out Prepare messages.

### Server Recovery

In theory, nearly all server data needs to be persistent. Instead of making you implement this, we simulate server failure by just disconnecting servers from the network (instead of crashing them). This means you **don't** need to implement any special logic for server recovery.

## The Code

You'll be implementing both the transaction coordinator and the servers. Code for the coordinator lives in `coordinator.go`, code for the servers lives in `server.go`, and data structures shared between them live in `3pc.go`. 

### Client Interface

The coordinator and servers both have methods that are meant to be called by clients (which the tester will simulate). These method signatures are provided in the skeleton code, along with comments to remind you what they should do.

``` go
// --- Coordinator --- 

// Initialize new Coordinator
//
// This will be called at the beginning of a test to create a new Coordinator
// It will also be called when the Coordinator restarts, so you'll need to trigger recovery here
// respChan is how you'll send messages to the client to notify it of committed or aborted transactions
func MakeCoordinator(servers []*labrpc.ClientEnd, respChan chan ResponseMsg) *Coordinator

// Start the 3PC protocol for a particular transaction
// TID is a unique transaction ID generated by the client
// This may be called concurrently. Ensure that any shared data is properly synchronized to handle concurrent calls.
func (co *Coordinator) FinishTransaction(tid int)

// Responses to the client
type ResponseMsg struct {
    tid        int
    committed  bool
    readValues map[string]interface{}
}
```

``` go
// --- Server ---

// Initialize new Server
//
// keys is a slice of the keys that this server is responsible for storing
func MakeServer(keys []string) *Server

// This function should log a Get operation
func (sv *Server) Get(tid int, key string)

// This function should log a Set operation
func (sv *Server) Set(tid int, key string, value interface{})
```

### RPC Interface

We provide headers and wrapper methods for the RPCs that you'll need to send from the coordinator to the servers. However, you'll need to fill in the reply structs in `3pc.go` with whatever fields those RPCs need to respond with. If `args` have type `struct{}`, the RPC doesn't need arguments and you can ignore that argument. Similarly, if `reply` has type `*struct{}`, the RPC doesn't need a reply, and you can ignore that argument. 

``` go
func (sv *Server) Prepare(args *RPCArgs, reply *PrepareReply)

func (sv *Server) Abort(args *RPCArgs, reply *struct{})

func (sv *Server) Query(args struct{}, reply *QueryReply)

func (sv *Server) PreCommit(args *RPCArgs, reply *struct{})

func (sv *Server) Commit(args *RPCArgs, reply *CommitReply)
```


## Testing

The tester will call your coordinator and servers with a series of operations. It will also call your coordinator with a series of FinishTransaction messages to finish transactions. Make sure to pass all tests before submitting. Output will be something like this:

``` 
$ go test -v -race

=== RUN   TestBasicCommit
TestBasicCommit: Commit when everything is fine ...
  ... Passed --   0.0  3   21    2076    0
--- PASS: TestBasicCommit (0.02s)
=== RUN   TestBasicAbort
TestBasicAbort: Abort when a server is disconnected ...
  ... Passed --   0.1  3   29    2506    0
--- PASS: TestBasicAbort (0.08s)
=== RUN   TestEasyRecovery
TestEasyRecovery: Recovery between transactions ...
  ... Passed --   0.0  3   24    2553    0
--- PASS: TestEasyRecovery (0.02s)
=== RUN   TestRelevance
TestRelevance: Transaction can still continue if irrelevant server is disconnected after Prepare ...
  ... Passed --   0.0  3   10    1068    0
--- PASS: TestRelevance (0.01s)
=== RUN   TestConcurrentDifferentKeys
TestConcurrentDifferentKeys: Transactions that don't touch the same keys can always succeed concurrently ...
  ... Passed --   0.1  3  153   13806    0
--- PASS: TestConcurrentDifferentKeys (0.11s)
=== RUN   TestConcurrentReadSameKeys
TestConcurrentReadSameKeys: Transactions that only read the same keys can always succeed concurrently ...
  ... Passed --   0.1  3  153   13892    0
--- PASS: TestConcurrentReadSameKeys (0.11s)
=== RUN   TestConcurrentWriteSameKeys
TestConcurrentWriteSameKeys: Concurrent transactions that write to the same keys should have at least one commit ...
  ... Passed --   0.1  3  173   13282    0
--- PASS: TestConcurrentWriteSameKeys (0.11s)
=== RUN   TestSerializability
TestSerializability: Concurrent transactions are executed serially ...
  ... Passed --   0.2  3  291   23423    0
--- PASS: TestSerializability (0.22s)
=== RUN   TestDisconnectPreCommit
TestDisconnectPreCommit: If a server disconnects before PreCommit, we abort ...
  ... Passed --   0.1  3   12     995    0
--- PASS: TestDisconnectPreCommit (0.07s)
=== RUN   TestDisconnectCommit
TestDisconnectCommit: If a server disconnects before Commit, we block until it returns ...
  ... Passed --   0.1  3   13    1279    0
--- PASS: TestDisconnectCommit (0.08s)
=== RUN   TestRestartPreCommit
TestRestartPreCommit: If the coordinator restarts before PreCommit, we commit ...
  ... Passed --   0.0  3   21    2088    0
--- PASS: TestRestartPreCommit (0.01s)
=== RUN   TestRestartCommit
TestRestartCommit: If the coordinator restarts before Commit, we commit ...
  ... Passed --   0.1  3  180   16269    0
--- PASS: TestRestartCommit (0.11s)
=== RUN   TestRestartMidPreCommit
TestRestartPreCommit: If the coordinator restarts in the middle of PreCommit, we commit ...
  ... Passed --   0.1  3  162   15534    0
--- PASS: TestRestartMidPreCommit (0.11s)
=== RUN   TestRestartMidCommit
TestRestartCommit: If the coordinator restarts in the middle of Commit, we commit ...
  ... Passed --   0.1  3  144   13419    0
--- PASS: TestRestartMidCommit (0.11s)
PASS
ok  	cs351/a6-3pc	2.182s
```

