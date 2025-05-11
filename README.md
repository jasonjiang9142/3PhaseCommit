
# Three-Phase Commit (3PC) Implementation


# Overview

This project implements the Three-Phase Commit (3PC) protocol in distributed systems. 3PC is an extension of the original Two-Phase Commit (2PC) protocol, for ensuring atomic transactions on a distributed key-value store. The primary advantage of 3PC over 2PC is that the transaction coordinator does not need to maintain persistent state, allowing any server to potentially take over as coordinator in case of failure. This implementation handles coordinator crashes and recovery, ensuring robust transaction management in a distributed environment.


## Features

- **Atomic Transactions:** Supports sequences of Get and Set operations as atomic units.
- **Coordinator Recovery:** Recovers from coordinator crashes by querying participating servers.
- **Concurrency Control:** Uses `sync.RWMutex` for concurrent key access.
- **Non-Persistent Servers:** Simulates failure via disconnection, not crashes.
- **Robust Testing:** Comprehensive suite for commits, aborts, recovery, and serializability.

---

## Protocol Description

### **0. Client Operations**
- Clients submit `Get` and `Set` operations to servers (logged but not executed).
- Clients call `FinishTransaction` on the coordinator to begin 3PC.

### **1. Prepare Phase**
- The coordinator sends `Prepare` messages to all servers.
- Servers attempt to lock keys involved in the transaction. If a server has no operations for the transaction, it marks itself as irrelevant.
- Servers vote `Yes` if locks are acquired, otherwise `No`. If any server votes `No` or times out, the coordinator aborts the transaction.

### **2. PreCommit Phase**
- If all votes are Yes, coordinator sends `PreCommit`.
- Servers acknowledge, or coordinator aborts on timeout.

### **3. Commit Phase**
- If all `PreCommit` messages are acknowledged, the coordinator sends `Commit` messages to relevant servers.
- Servers execute the logged operations and return `Get` operation values.
- The coordinator retries `Commit` messages on timeout until servers respond.
- The coordinator notifies the client of the committed transaction and returns `Get` values.

### Abort Handling
- If the coordinator decides to `abort` (e.g., due to a `No` vote or timeout), it sends `Abort` messages to all servers and informs the client.

### Coordinator Recovery
- On restart, the coordinator sends Query messages to all servers to determine transaction states.
- Based on server responses, the coordinator:
    - Aborts transactions voted No or already aborted.
    - Resumes transactions at the Commit phase if some servers have committed.
    - Resumes at the PreCommit phase if any server has pre-committed.
    - Resumes at the Prepare phase if any server has voted Yes.



---

## Code Structure

| File            | Description                                      |
|-----------------|--------------------------------------------------|
| `coordinator.go`| 3PC coordinator logic and recovery               |
| `server.go`     | Server logic, logging, and locking               |
| `3pc.go`        | Shared data structures and RPC definitions       

---

## Client Interface

### Coordinator
- `MakeCoordinator()`: Initializes a new coordinator, triggering recovery if restarted.
- `FinishTransaction(txnID)`: Starts the 3PC protocol for a given transaction ID.
- `ResponseMsg`: Struct for client responses, including transaction ID, commit status, and `Get` operation values.

### Server
- `MakeServer(keys)`: Initializes a server with a list of managed keys.
- `Get(txnID, key)`: Logs a Get operation for a transaction.
- `Set(txnID, key, val)`: Logs a Set operation for a transaction.

---

## RPC Interface
The coordinator communicates with servers via the following RPCs:

- `Prepare`: Initiates the prepare phase, with servers responding with their vote.
- `PreCommit`: Requests acknowledgment for the pre-commit phase.
- `Commit`: Instructs servers to execute operations, returning Get values.
- `Abort`: Notifies servers to abort a transaction.
- `Query`: Retrieves transaction states during coordinator recovery.

---

## Concurrency

Concurrency is managed via `sync.RWMutex`:
- `Set` uses `Lock()` for exclusive access.
- `Get` uses `RLock()` for concurrent reads.
- Methods like `Lock()`, `Unlock()`, `RLock()`, and `RUnlock()` ensure thread-safe key access.


---

## Testing

The project includes a comprehensive test suite to validate functionality:

- **Basic Tests:** Verify commits and aborts under normal and failure conditions.
- **Recovery Tests:** Ensure correct coordinator recovery after crashes.
- **Concurrency Tests:** Validate concurrent transaction handling for different and same keys.
- **Serializability Tests:** Confirm transactions are executed serially when required.
- **Disconnection Tests:** Test behavior when servers disconnect during various phases.

Example test output:
```bash
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



## Setup and Installation

Clone the project

**Prerequisites:**

- Go (version 1.16 or later)
- Git

**Clone the Repository:**
```bash
git clone https://github.com/your-username/three-phase-commit.git
cd three-phase-commit
```

**Run Tests:**

```bash
  go test -v -race
```

## Usage

To use this implementation in a distributed system:
- Initialize servers with their respective keys using MakeServer.
- Create a coordinator with a list of server endpoints using MakeCoordinator.
- Clients can submit Get and Set operations to servers and call FinishTransaction on the coordinator to commit transactions.

## Limitations

- Server persistence is not implemented; failures are simulated via network disconnection.
- The implementation assumes reliable RPC communication with retries on timeouts.

## Contributing

Contributions are welcome! Please submit a pull request or open an issue for bugs, improvements, or feature requests.

## License


This project is licensed under the 
[MIT](https://choosealicense.com/licenses/mit/) License.
