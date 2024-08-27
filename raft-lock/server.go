package raft_lock

import (
	"cs350/labgob"
	"cs350/labrpc"
	"cs350/raft"
	"log"
	"sync"
	"sync/atomic"
	"time"
)

const Debug = 0

func DPrintf(format string, a ...interface{}) (n int, err error) {
	if Debug > 0 {
		log.Printf(format, a...)
	}
	return
}

type Op struct {
	// Your definitions here.
	// Field names must start with capital letters,
	// otherwise RPC will break.
	Type OpType
	Key  string
	// duplicate detection info needs to be part of state machine
	// so that all raft servers eliminate the same duplicates
	ClientId  ClientID
	RequestId int64
}

type LockServer struct {
	clerkEnds   map[ClientID]*labrpc.ClientEnd
	events      chan Event
	mu          sync.Mutex
	me          int
	rf          *raft.Raft
	applyCh     chan raft.ApplyMsg
	dead        int32 // set by Kill()
	store       LockStore[string, ClientID]
	results     map[int]chan Op
	lastApplied map[int64]int64
}

// LockUnlock RPC handler
func (ls *LockServer) Mutex(args *MutexArgs, reply *MutexReply) {
	// Your code here.
	op := Op{
		Type:      args.Op,
		Key:       args.Key,
		ClientId:  args.ClientId,
		RequestId: args.RequestId,
	}

	ok, _ := ls.waitForApplied(op)
	if !ok {
		reply.Err = ErrWrongLeader
		return
	}

	reply.Err = OK
}

// send the op log to Raft library and wait for it to be applied
func (ls *LockServer) waitForApplied(op Op) (bool, Op) {
	index, _, isLeader := ls.rf.Start(op)

	if !isLeader {
		// fmt.Printf("debug 5 wrong leader return\n")
		return false, op
	}

	ls.mu.Lock()
	opCh, ok := ls.results[index]
	if !ok {
		opCh = make(chan Op, 1)
		ls.results[index] = opCh
	}
	ls.mu.Unlock()

	select {
	case appliedOp := <-opCh:
		return ls.isSameOp(op, appliedOp), appliedOp
	case <-time.After(20 * time.Millisecond):
		return false, op
	}
}

// check if the issued command is the same as the applied command
func (ls *LockServer) isSameOp(issued Op, applied Op) bool {
	return issued.ClientId == applied.ClientId && issued.RequestId == applied.RequestId
}

// background loop to receive the logs committed by the Raft
// library and apply them to the kv server state machine
func (ls *LockServer) applyOpsLoop() {
	for !ls.killed() {
		msg := <-ls.applyCh
		if !msg.CommandValid {
			continue
		}
		index := msg.CommandIndex
		op := msg.Command.(Op)

		ls.mu.Lock()

		lastId, ok := ls.lastApplied[op.ClientId]
		if !ok || op.RequestId > lastId {
			ls.applyToStateMachine(&op)
			ls.lastApplied[op.ClientId] = op.RequestId
		}

		opCh, ok := ls.results[index]
		if !ok {
			opCh = make(chan Op, 1)
			ls.results[index] = opCh
		}
		opCh <- op
		ls.mu.Unlock()
	}
}

func (ls *LockServer) isLeader() bool {
	_, isLeader := ls.rf.GetState()
	return isLeader
}

func (ls *LockServer) giveLockToOwner(key string, lockOwner ClientID) {
	if ls.isLeader() {
		// DPrintf("The leader gave %d the lock %s!\n", lockOwner, key)
		args := ReceiveLockArgs{Key: key}
		reply := ReceiveLockReply{}
		if endpoint, exists := ls.clerkEnds[lockOwner]; exists {
			endpoint.Call("Clerk.ReceiveLock", &args, &reply)
		} else {
			DPrintf("missing clerk %v", lockOwner)
		}
	}
}

// applied the command to the state machine
// lock must be held before calling this
//
// Fetches the existing lock owner for the given key
// from the lock store,
// then applies the new operation to the lock store
// sends an Event to the ls.events channel
// and then calls ls.giveLockToOwner() if ownership has changed

func (ls *LockServer) applyToStateMachine(op *Op) {
	// 4C: Your code here
	owner, _ := ls.store.getLockOwner(op.Key)

	if op.Type == LockOp {
		ls.store.lock(op.Key, op.ClientId)
	}

	if op.Type == UnlockOp {
		ls.store.unlock(op.Key, op.ClientId)
	}

	if ls.isLeader() {
		ls.events <- Event{Op: op.Type, eventType: ServerEvent, key: op.Key, id: op.ClientId}
		newOwner, _ := ls.store.getLockOwner(op.Key)
		if newOwner != owner {
			ls.giveLockToOwner(op.Key, newOwner)
		}
	}

}

// the tester calls Kill() when a LockServer instance won't
// be needed again. for your convenience, we supply
// code to set rf.dead (without needing a lock),
// and a killed() method to test rf.dead in
// long-running loops. you can also add your own
// code to Kill(). you're not required to do anything
// about this, but it may be convenient (for example)
// to suppress debug output from a Kill()ed instance.
func (ls *LockServer) Kill() {
	atomic.StoreInt32(&ls.dead, 1)
	ls.rf.Kill()
	// Your code here, if desired.
}

func (ls *LockServer) killed() bool {
	z := atomic.LoadInt32(&ls.dead)
	return z == 1
}

// servers[] contains the ports of the set of
// servers that will cooperate via Raft to
// form the fault-tolerant key/value service.
// me is the index of the current server in servers[].
// StartLockServer() must return quickly, so it should start goroutines
// for any long-running work.
func StartLockServer(servers []*labrpc.ClientEnd, clerkEnds map[ClientID]*labrpc.ClientEnd, events chan Event, me int, persister *raft.Persister) *LockServer {
	// call labgob.Register on structures you want
	// Go's RPC library to marshall/unmarshall.
	labgob.Register(Op{})

	lock := new(LockServer)
	lock.me = me

	// You may need initialization code here.
	lock.applyCh = make(chan raft.ApplyMsg, 1)
	lock.rf = raft.Make(servers, me, persister, lock.applyCh)
	lock.store = make(LockStore[string, ClientID])
	lock.clerkEnds = clerkEnds
	lock.events = events
	lock.results = make(map[int]chan Op)
	lock.lastApplied = make(map[int64]int64)

	// You may need initialization code here.
	go lock.applyOpsLoop()

	return lock
}
