package commit

import (
	"log"
	"sync"
)

type StoreItem struct {
	value interface{}
	lock  sync.RWMutex

	// Any extra fields here

}

type Server struct {
	mu    sync.Mutex
	store map[string]*StoreItem

	// Your fields here
	operations map[int][]Operation
	states     map[int]TransactionState
}

// Prepare handler

//

// This function should:

// 1. Attempt to obtain locks for the given transaction

// 2. If this succeeds, vote Yes

// 3. If this fails, release any obtained locks and vote No

func (sv *Server) Prepare(args *RPCArgs, reply *PrepareReply) {

	log.Printf("Prepare")
	// log.Printf("Aquiring prepare lock")
	// sv.mu.Lock()
	// log.Printf("Aquired prepare lock")
	// defer sv.mu.Unlock()

	// if the transaction ID is already committed, set the reply to false

	tId := args.Tid // get the transaction ID from the args

	sv.mu.Lock()
	ops, exists := sv.operations[tId] // check if the transaction ID exists in the operations map

	if !exists || len(ops) == 0 {
		sv.mu.Unlock()
		reply.Relevant = false // if the transaction ID does not exist, set the reply to false
		return
	}

	sv.mu.Unlock()

	reply.Relevant = true
	reply.Vote = true

	sv.mu.Lock()
	// check if the transaction ID exists in the states map
	if sv.states[args.Tid] != stateOperations {
		log.Printf("Prepare: transaction ID %d already exists", args.Tid)
		if sv.states[args.Tid] == stateVotedYes || sv.states[args.Tid] == statePreCommitted || sv.states[args.Tid] == stateCommitted {
			reply.Vote = true
		} else {
			reply.Vote = false
		}
		sv.mu.Unlock()
		log.Printf("Prepare: transaction ID %d already exists", args.Tid)
		return
	}
	sv.mu.Unlock()

	locks := make([]*StoreItem, 0)

	// try to obtain locks for all the operations
	log.Printf("Prepare: try to obtain locks for all the operations")
	for _, op := range ops {
		sv.mu.Lock()
		item, exist := sv.store[op.Key]
		sv.mu.Unlock()
		log.Printf("Prepare: lock obtained for key %s", op.Key)

		// if the item does not exist, set the reply to false
		if !exist {
			log.Printf("Prepare: item does not exist")
			reply.Vote = false
			sv.mu.Lock()
			sv.states[tId] = stateVotedNo
			sv.mu.Unlock()

			// unlock all the locks obtained so far
			for _, lock := range locks {
				if lock != nil {
					lock.lock.Unlock()
				}
			}

			return
		}

		log.Printf("Prepare: item exists for key %s", op.Key)

		// try to obtain the lock for the item
		if op.IsGet {
			log.Printf("Prepare: read lock obtained for key %s", op.Key)
			item.lock.RLock() // use read lock for get operation
			log.Printf("Prepare: finished read lock obtained for key %s", op.Key)

		} else {
			log.Printf("Prepare: write lock obtained for key %s", op.Key)
			item.lock.Lock() // use write lock for set operation
			log.Printf("Prepare: finished write lock obtained for key %s", op.Key)

		}
		log.Printf("Prepare: lock obtained for key %s after trying to obtain the lock", op.Key)

		locks = append(locks, item) // add the lock to the list of locks obtained

	}

	log.Printf("Prepare: locks obtained for all operations")

	sv.mu.Lock()
	sv.states[tId] = stateVotedYes
	sv.mu.Unlock()
}

// Abort handler
// This function should abort the given transaction
// Make sure to release any held locks

func (sv *Server) Abort(args *RPCArgs, reply *struct{}) {

	log.Printf("Abort")

	// log.Printf("Aquiring abort lock")
	sv.mu.Lock()
	// log.Printf("Aquired abort lock")

	defer sv.mu.Unlock()

	tId := args.Tid // get the transaction ID from the args
	// check if the transaction ID exists in the states map

	if _, exists := sv.states[tId]; !exists || sv.states[tId] == stateAborted {
		return

	}

	// release all locks obtained for the transaction

	log.Printf("Releasing abort locks")

	for _, op := range sv.operations[tId] {
		item, exist := sv.store[op.Key]
		if exist {
			if op.IsGet {
				log.Printf("Releasing read lock")
				item.lock.RUnlock() // use read unlock for get operation
			} else {
				log.Printf("Releasing write lock")
				item.lock.Unlock() // use write unlock for set operation
			}
		}
	}

	sv.states[tId] = stateAborted // set the state to aborted
	// delete(sv.operations, tId)    // delete the operations for the transaction ID
	log.Printf("Transaction %d: server finished aborting", tId) // log the operation

}

// Query handler

//

// This function should reply with information about all known transactions

func (sv *Server) Query(args struct{}, reply *QueryReply) {

	log.Printf("Query")
	// log.Printf("Aquiring query lock")
	sv.mu.Lock()
	// log.Printf("Aquired query lock")
	defer sv.mu.Unlock()

	reply.Transactions = make(map[int]ServerTransaction)
	for tid, state := range sv.states {
		reply.Transactions[tid] = ServerTransaction{
			State:      state,
			Operations: sv.operations[tid],
		}

	}

}

// PreCommit handler

//

// This function should confirm that the server is ready to commit

// Hint: the protocol tells us to always just acknowledge preCommit,

// so there isn't too much to do here

func (sv *Server) PreCommit(args *RPCArgs, reply *struct{}) {

	log.Printf("Server: Handling PreCommit for transaction %d", args.Tid)
	// log.Printf("Aquiring preCommit lock")
	sv.mu.Lock()
	// log.Printf("Aquired preCommit lock")
	defer sv.mu.Unlock()

	tid := args.Tid // get the transaction ID from the args
	// check if the transaction ID exists in the states map
	if _, exists := sv.operations[tid]; exists && sv.states[tid] == stateVotedYes {
		sv.states[tid] = statePreCommitted
	}

	log.Printf("Server: Finished PreCommit for transaction %d", args.Tid)

}

// Commit handler

//

// This function should actually apply the logged operations

// Make sure to release any held locks

func (sv *Server) Commit(args *RPCArgs, reply *CommitReply) {

	log.Printf("Commit")
	// log.Printf("Aquiring commit lock")
	sv.mu.Lock()
	// log.Printf("Aquired commit lock")
	defer sv.mu.Unlock()

	tid := args.Tid // get the transaction ID from the args
	ops, exists := sv.operations[tid]
	reply.ReadValues = make(map[string]interface{})
	if !exists || sv.states[tid] != statePreCommitted {
		return
	}

	// apply the operations and unlock the locks

	for _, op := range ops {
		item, exist := sv.store[op.Key]

		if exist {
			if op.IsGet {
				reply.ReadValues[op.Key] = item.value                         // get the value for the key
				log.Printf("Transaction %d: server finished committing", tid) // log the operation
				item.lock.RUnlock()                                           // use read unlock for get operation

			} else {
				item.value = op.Value                                         // set the value for the key
				log.Printf("Transaction %d: server finished committing", tid) // log the operation
				item.lock.Unlock()                                            // use write unlock for set operation

			}

		}

	}

	sv.states[tid] = stateCommitted // set the state to committed

	// delete(sv.operations, tid) // delete the operations for the transaction ID

}

// Get

//

// This function should log a Get operation

func (sv *Server) Get(tid int, key string) {

	log.Printf("Get")
	// log.Printf("Aquiring get lock")
	sv.mu.Lock()
	// log.Printf("Aquired get lock")
	log.Printf("Get complete")
	defer sv.mu.Unlock()

	// append the log to the operations
	sv.operations[tid] = append(sv.operations[tid], Operation{
		IsGet: true,
		Key:   key})

	// // if the key doesn't exist yet, create a new state for the key to be set
	// if _, exists := sv.states[tid]; !exists {
	// 	sv.states[tid] = stateOperations
	// }

}

// Set

//

// This function should log a Set operation

func (sv *Server) Set(tid int, key string, value interface{}) {

	log.Printf("Set")
	// log.Printf("Aquiring set lock")
	sv.mu.Lock()
	// log.Printf("Aquired set lock")
	defer sv.mu.Unlock()

	// append the log to the operations along with the set value

	sv.operations[tid] = append(sv.operations[tid], Operation{
		IsGet: false,
		Key:   key,
		Value: value})

	// if the key doesn't exist yet, create a new state for the key to be set
	// if _, exists := sv.states[tid]; !exists {
	// 	sv.states[tid] = stateOperations

	// }

}

// Initialize new Server

//

// keys is a slice of the keys that this server is responsible for storing

func MakeServer(keys []string) *Server {

	sv := &Server{
		// Initialize fields here
		store:      make(map[string]*StoreItem),
		operations: make(map[int][]Operation),
		states:     make(map[int]TransactionState),
	}

	// Initialize the store with the keys
	for _, key := range keys {
		sv.store[key] = &StoreItem{
			value: nil,
		}

	}

	return sv

}
