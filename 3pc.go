package commit

// ------------------------------------------
//                  COMMON
// ------------------------------------------

type TransactionState int

const (
	stateOperations TransactionState = iota
	stateVotedNo
	stateVotedYes
	statePreCommitted
	stateAborted
	stateCommitted
)

const (
	PhasePrepare   = "Prepare"
	PhasePreCommit = "PreCommit"
	PhaseCommitted = "Committed"
	PhaseAborted   = "Aborted"
)

// Common args struct because RPCs generally have the transaction ID as their only argument
type RPCArgs struct {
	Tid int
}

// PrepareReply struct to hold the response of the prepare phase
// used in the prepare phase to determine if the server is relavent to the transaction
type PrepareReply struct {
	// Your fields here
	Relevant bool // True if the server is relavant to the transaction
	Vote     bool // True if the server is willing to vote yes
}

// response to the rpc query with current state of the transaction
// used in the query phase to determine the state of the transaction
type QueryReply struct {
	Transactions map[int]ServerTransaction // transaction ID : state of the transaction
}

// CommitReply struct to hold the response of the commit phase
// used in the commit phase to determine if the server is relavent to the transaction
type CommitReply struct {
	// Your fields here
	ReadValues map[string]interface{} // name of data : value of data
}

// represents the operation to be performed in the transaction
// used to define opeartions in the transaction
type Operation struct {
	IsGet bool        // true if the operation is a get, false if it is a set
	Key   string      // key of the operation
	Value interface{} // Value for Set (nil for Get)
}

// Transaction struct to hold the state of a transaction
// used to track progress and operation of a transaction on the server
type ServerTransaction struct {
	State      TransactionState // State of the transaction
	Operations []Operation      // Operations to be performed in the transaction
}
