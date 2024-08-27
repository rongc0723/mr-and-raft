package raft

//
// This is an outline of the API that raft must expose to
// the service (or tester). See comments below for
// each of these functions for more details.
//
// rf = Make(...)
//   Create a new Raft server.
// rf.Start(command interface{}) (index, term, isleader)
//   Start agreement on a new log entry
// rf.GetState() (term, isLeader)
//   ask a Raft for its current term, and whether it thinks it is leader
// ApplyMsg
//   Each time a new entry is committed to the log, each Raft peer
//   should send an ApplyMsg to the service (or tester) in the same server.
//

import (
	"bytes"
	"cs350/labgob"
	"cs350/labrpc"
	"math/rand"
	"sync"
	"sync/atomic"
	"time"
)

// As each Raft peer becomes aware that successive log entries are
// committed, the peer should send an ApplyMsg to the service (or
// tester) on the same server, via the applyCh passed to Make(). Set
// CommandValid to true to indicate that the ApplyMsg contains a newly
// committed log entry.
type ApplyMsg struct {
	CommandValid bool
	Command      interface{}
	CommandIndex int
}

// A Go object implementing a single Raft peer.
type Raft struct {
	mu        sync.Mutex          // Lock to protect shared access to this peer's state
	peers     []*labrpc.ClientEnd // RPC end points of all peers
	persister *Persister          // Object to hold this peer's persisted state
	me        int                 // This peer's index into peers[]
	dead      int32               // Set by Kill()

	// Your data here (4A, 4B).
	// Look at the paper's Figure 2 for a description of what
	// state a Raft server must maintain.

	// election states
	state            State
	votes            int
	heartBeatChan    chan bool
	receivedVoteChan chan bool

	//persistent state on all servers
	currentTerm int
	votedFor    int
	log         []LogEntry

	// volatile state on all servers
	commitIndex int
	lastApplied int

	// volatile state on leaders
	nextIndex  []int
	matchIndex []int

	applyCh chan ApplyMsg
}

type LogEntry struct {
	Term    int
	Command interface{}
}

// Return currentTerm and whether this server
// believes it is the leader.
func (rf *Raft) GetState() (int, bool) {
	var term int
	var isleader bool
	// Your code here (4A).
	rf.mu.Lock()
	term = rf.currentTerm
	isleader = rf.state == Leader
	rf.mu.Unlock()
	return term, isleader
}

// Save Raft's persistent state to stable storage, where it
// can later be retrieved after a crash and restart. See paper's
// Figure 2 for a description of what should be persistent.
func (rf *Raft) persist() {
	// Your code here (4B).
	// Example:
	w := new(bytes.Buffer)
	e := labgob.NewEncoder(w)
	e.Encode(rf.log)
	e.Encode(rf.currentTerm)
	e.Encode(rf.votedFor)
	data := w.Bytes()
	rf.persister.SaveRaftState(data)
}

// Restore previously persisted state.
func (rf *Raft) readPersist(data []byte) {
	if data == nil || len(data) < 1 { // bootstrap without any state?
		return
	}
	// Your code here (4B).
	// Example:
	r := bytes.NewBuffer(data)
	d := labgob.NewDecoder(r)
	if d.Decode(&rf.log) != nil ||
		d.Decode(&rf.currentTerm) != nil || d.Decode(&rf.votedFor) != nil {
		panic("error reading persisted state")
	}
}

// The service using Raft (e.g. a k/v server) wants to start
// agreement on the next command to be appended to Raft's log. If this
// server isn't the leader, returns false. Otherwise start the
// agreement and return immediately. There is no guarantee that this
// command will ever be committed to the Raft log, since the leader
// may fail or lose an election. Even if the Raft instance has been killed,
// this function should return gracefully.
//
// The first return value is the index that the command will appear at
// if it's ever committed. The second return value is the current
// term. The third return value is true if this server believes it is
// the leader.
func (rf *Raft) Start(command interface{}) (int, int, bool) {
	index := -1
	term := -1
	isLeader := true

	// Your code here (4B).
	rf.mu.Lock()

	term = rf.currentTerm

	if rf.state != Leader {
		isLeader = false
		rf.mu.Unlock()
		return index, term, isLeader
	}

	rf.log = append(rf.log, LogEntry{term, command})
	index = len(rf.log) - 1
	rf.matchIndex[rf.me] = index
	rf.nextIndex[rf.me] = index + 1
	// fmt.Println("leader", rf.me, "received command", command, "for index", index)
	rf.mu.Unlock()
	return index, term, isLeader
}

// The tester doesn't halt goroutines created by Raft after each test,
// but it does call the Kill() method. Your code can use killed() to
// check whether Kill() has been called. The use of atomic avoids the
// need for a lock.
//
// The issue is that long-running goroutines use memory and may chew
// up CPU time, perhaps causing later tests to fail and generating
// confusing debug output. Any goroutine with a long-running loop
// should call killed() to check whether it should stop.
func (rf *Raft) Kill() {
	atomic.StoreInt32(&rf.dead, 1)
	// Your code here, if desired.
}

func (rf *Raft) killed() bool {
	z := atomic.LoadInt32(&rf.dead)
	return z == 1
}

type State int

const (
	Leader State = iota
	Follower
	Candidate
)

// The ticker go routine starts a new election if this peer hasn't received
// heartsbeats recently.
func (rf *Raft) ticker() {

	// while server is alive
	for rf.killed() == false {
		// Your code here to check if a leader election should
		// be started and to randomize sleeping time using
		time.Sleep(10 * time.Millisecond)
		rf.mu.Lock()
		state := rf.state
		rf.persist()
		rf.mu.Unlock()

		switch state {
		case Follower:
			electionTimeOut := time.Duration(rand.Intn(150)+200) * time.Millisecond
			select {
			case <-rf.receivedVoteChan:

			case <-rf.heartBeatChan:

			case <-time.After(electionTimeOut):
				rf.becomeCandidate()
			}
		case Candidate:
			electionTimeOut := time.Duration(rand.Intn(150)+200) * time.Millisecond
			select {
			case <-rf.receivedVoteChan:

			case <-rf.heartBeatChan:

			case <-time.After(electionTimeOut):
				rf.becomeCandidate()
			}

		case Leader:
			select {
			case <-rf.heartBeatChan:

			case <-time.After(50 * time.Millisecond):
				rf.sendHeartBeats()
			}
		}

	}
}

func (rf *Raft) becomeCandidate() {
	rf.mu.Lock()
	rf.state = Candidate
	rf.currentTerm++
	rf.votedFor = rf.me
	rf.votes = 1

	args := RequestVoteArgs{
		Term:         rf.currentTerm,
		CandidateId:  rf.me,
		LastLogIndex: len(rf.log) - 1,
		LastLogTerm:  rf.log[len(rf.log)-1].Term,
	}

	for peer := range rf.peers {
		if peer != rf.me {
			go rf.sendRequestVote(peer, &args, &RequestVoteReply{})
		}
	}
	rf.mu.Unlock()
}

// Example RequestVote RPC arguments structure.
// Field names must start with capital letters!
type RequestVoteArgs struct {
	// Your data here (4A, 4B).
	Term         int
	CandidateId  int
	LastLogIndex int
	LastLogTerm  int
}

// Example RequestVote RPC reply structure.
// Field names must start with capital letters!
type RequestVoteReply struct {
	// Your data here (4A).
	Term        int
	VoteGranted bool
}

// Example RequestVote RPC handler.
func (rf *Raft) RequestVote(args *RequestVoteArgs, reply *RequestVoteReply) {
	// Your code here (4A, 4B).

	// args is the candidate's term and id
	// reply is the peer's term and voteGranted

	rf.mu.Lock()

	if args.Term < rf.currentTerm {
		reply.Term = rf.currentTerm
		reply.VoteGranted = false
		rf.mu.Unlock()
		rf.receivedVoteChan <- true
		return
	}

	if args.Term > rf.currentTerm {
		rf.state = Follower
		rf.currentTerm = args.Term
		rf.votedFor = -1
	}

	lastIndex := len(rf.log) - 1
	lastTerm := rf.log[lastIndex].Term
	candidateUpToDate := false

	// if the candidate's last log term is the same as the peer's last log term
	// fmt.Println("Candidate", args.CandidateId, " last term", args.LastLogTerm)
	// fmt.Println("follower", rf.me, " last term", lastTerm)
	if args.LastLogTerm == lastTerm {
		// check if the candidate's last log index is at least peer's last log index
		if args.LastLogIndex >= lastIndex {
			candidateUpToDate = true
		}
		// fmt.Println(args.LastLogIndex, lastIndex)
		// fmt.Println("candidate up to date:", candidateUpToDate)
	} else {
		// if candidate has a diff last log term than the peer,
		// check if the candidate's last log term is greater than the peer's last log term
		if args.LastLogTerm > lastTerm {
			candidateUpToDate = true
		}
	}

	if (rf.votedFor == -1 || rf.votedFor == args.CandidateId) && candidateUpToDate {
		rf.votedFor = args.CandidateId
		reply.VoteGranted = true
	}

	rf.mu.Unlock()
	rf.receivedVoteChan <- true

}

// Example code to send a RequestVote RPC to a server.
// Server is the index of the target server in rf.peers[].
// Expects RPC arguments in args. Fills in *reply with RPC reply,
// so caller should pass &reply.
//
// The types of the args and reply passed to Call() must be
// the same as the types of the arguments declared in the
// handler function (including whether they are pointers).
//
// The labrpc package simulates a lossy network, in which servers
// may be unreachable, and in which requests and replies may be lost.
// Call() sends a request and waits for a reply. If a reply arrives
// within a timeout interval, Call() returns true; otherwise
// Call() returns false. Thus Call() may not return for a while.
// A false return can be caused by a dead server, a live server that
// can't be reached, a lost request, or a lost reply.
//
// Call() is guaranteed to return (perhaps after a delay) *except* if the
// handler function on the server side does not return.  Thus there
// is no need to implement your own timeouts around Call().
//
// Look at the comments in ../labrpc/labrpc.go for more details.
func (rf *Raft) sendRequestVote(server int, args *RequestVoteArgs, reply *RequestVoteReply) {
	ok := rf.peers[server].Call("Raft.RequestVote", args, reply)
	if !ok {
		return
	}
	rf.mu.Lock()

	if reply.Term > rf.currentTerm {
		rf.currentTerm = reply.Term
		rf.state = Follower
		rf.votedFor = -1
		rf.mu.Unlock()
		return
	}

	if reply.VoteGranted {
		rf.votes++
		if rf.votes > len(rf.peers)/2 {
			if rf.state == Candidate {
				rf.state = Leader
				// for each server, initialize the index of next log entry
				// to send to that server
				rf.nextIndex = make([]int, len(rf.peers))

				// for each server, index of highest log entry known to be
				// replicated on that server
				rf.matchIndex = make([]int, len(rf.peers))

				initialIndex := len(rf.log)
				for i := range rf.peers {
					// initialize nextIndex for each server to be leader's last log index + 1
					rf.nextIndex[i] = initialIndex
					// initialize matchIndex for each server to 0
					rf.matchIndex[i] = 0
				}
				args := &AppendEntriesArgs{}
				for peer := range rf.peers {
					if peer != rf.me {
						go rf.sendAppendEntries(peer, args, &AppendEntriesReply{})
					}
				}
			}
		}
	}
	rf.mu.Unlock()

}

type AppendEntriesArgs struct {
	Term         int
	LeaderId     int
	PrevLogIndex int
	PrevLogTerm  int
	Entries      []LogEntry
	LeaderCommit int
}

type AppendEntriesReply struct {
	Term    int
	Success bool
}

func (rf *Raft) AppendEntries(args *AppendEntriesArgs, reply *AppendEntriesReply) {
	rf.mu.Lock()

	// tell candidate to step down to follower
	if args.Term < rf.currentTerm {
		reply.Term = rf.currentTerm
		reply.Success = false
		rf.mu.Unlock()
		rf.heartBeatChan <- true
		return
	}

	// step down to a follower
	if args.Term > rf.currentTerm {
		rf.currentTerm = args.Term
		rf.state = Follower
		rf.votedFor = -1
	}

	reply.Term = rf.currentTerm
	reply.Success = false

	// if log doesn't contain an entry at prevLogIndex
	// whose term matches prevLogTerm reply false (follower log shorter)
	if args.PrevLogIndex >= len(rf.log) {
		// leader has longer log
		rf.mu.Unlock()
		rf.heartBeatChan <- true
		return
	}

	// if an existing entry conflicts with a new one
	// (same index but different terms), delete the
	// existing entry and all that follow it
	if args.PrevLogTerm != rf.log[args.PrevLogIndex].Term {
		rf.log = rf.log[:args.PrevLogIndex]
		rf.mu.Unlock()
		rf.heartBeatChan <- true
		return
	}

	//strip all entries in log of follower past PrevLogIndex
	rf.log = rf.log[:args.PrevLogIndex+1]
	// append any new entries not already in the log
	newLog := []LogEntry{}
	newLog = append(newLog, rf.log...)
	rf.log = append(newLog, args.Entries...)

	reply.Success = true
	// if leaderCommit > commitIndex
	// set commitIndex to min(leaderCommit, index of last new entry)
	if args.LeaderCommit > rf.commitIndex {
		if args.LeaderCommit > len(rf.log)-1 {
			rf.commitIndex = len(rf.log) - 1
		} else {
			rf.commitIndex = args.LeaderCommit
		}
		for startIndex := rf.lastApplied + 1; startIndex <= rf.commitIndex; startIndex++ {
			rf.applyCh <- ApplyMsg{true, rf.log[startIndex].Command, startIndex}
			rf.lastApplied = startIndex
		}
	}

	rf.mu.Unlock()
	rf.heartBeatChan <- true

}

func (rf *Raft) sendAppendEntries(server int, args *AppendEntriesArgs, reply *AppendEntriesReply) {
	ok := rf.peers[server].Call("Raft.AppendEntries", args, reply)

	if !ok {
		return
	}

	rf.mu.Lock()

	if rf.state != Leader {
		rf.mu.Unlock()
		return
	}

	if reply.Term > rf.currentTerm {
		rf.currentTerm = reply.Term
		rf.state = Follower
		rf.votedFor = -1
		rf.mu.Unlock()
		return
	}

	if !reply.Success && rf.nextIndex[server] > 1 {
		rf.nextIndex[server] = rf.nextIndex[server] / 2
		rf.mu.Unlock()
		return
	}

	if reply.Success {
		rf.matchIndex[server] = len(args.Entries) + args.PrevLogIndex
		rf.nextIndex[server] = rf.matchIndex[server] + 1
	}

	//if there exists an N such tht N > commitIndex, a majority of matchIndex[i] >= N,
	//and log[N].term == currentTerm: set commitIndex = N
	for N := len(rf.log) - 1; N >= rf.commitIndex; N-- {
		count := 0
		// check if a majority of servers has replicated the log entry up to atleast N'th index
		if rf.log[N].Term == rf.currentTerm {
			for _, idx := range rf.matchIndex {
				if idx >= N {
					count++
				}
			}
		}

		// if majority of servers have replicated the log entry up to atleast N'th index
		// set commitIndex to N and start committing the log entries up to commitIndex
		// from lastApplied
		if count > len(rf.peers)/2 && rf.state == Leader {
			rf.commitIndex = N
			for startIndex := rf.lastApplied + 1; startIndex <= rf.commitIndex; startIndex++ {
				rf.applyCh <- ApplyMsg{true, rf.log[startIndex].Command, startIndex}
				rf.lastApplied = startIndex
			}
			break
		}
	}
	rf.mu.Unlock()
	// fmt.Println("leader", rf.me, "next index for servers", rf.nextIndex)
}

func (rf *Raft) sendHeartBeats() {
	rf.mu.Lock()
	if rf.state != Leader {
		rf.mu.Unlock()
		return
	}

	for peer := range rf.peers {
		if peer != rf.me {
			args := &AppendEntriesArgs{
				Term:         rf.currentTerm,
				LeaderId:     rf.me,
				PrevLogIndex: rf.nextIndex[peer] - 1,
				LeaderCommit: rf.commitIndex,
			}
			if rf.nextIndex[peer] < len(rf.log) {
				args.PrevLogTerm = rf.log[rf.nextIndex[peer]-1].Term
				args.Entries = rf.log[rf.nextIndex[peer]:]
			} else {
				args.PrevLogTerm = rf.log[len(rf.log)-1].Term
				args.Entries = []LogEntry{}
			}
			// fmt.Println(args.Entries)
			go rf.sendAppendEntries(peer, args, &AppendEntriesReply{})
		}
	}
	rf.mu.Unlock()
}

// The service or tester wants to create a Raft server. The ports
// of all the Raft servers (including this one) are in peers[]. This
// server's port is peers[me]. All the servers' peers[] arrays
// have the same order. persister is a place for this server to
// save its persistent state, and also initially holds the most
// recent saved state, if any. applyCh is a channel on which the
// tester or service expects Raft to send ApplyMsg messages.
// Make() must return quickly, so it should start goroutines
// for any long-running work.
func Make(peers []*labrpc.ClientEnd, me int,
	persister *Persister, applyCh chan ApplyMsg) *Raft {
	rf := &Raft{}
	rf.peers = peers
	rf.persister = persister
	rf.me = me

	// Your initialization code here (4A, 4B).

	//all servers start as follower
	rf.state = Follower
	rf.currentTerm = 0
	rf.votedFor = -1
	rf.votes = 0
	rf.heartBeatChan = make(chan bool)
	rf.receivedVoteChan = make(chan bool)
	rf.commitIndex = 0
	rf.lastApplied = 0
	newLog := []LogEntry{}
	newLog = append(newLog, LogEntry{Term: 0})
	rf.log = newLog
	rf.applyCh = applyCh

	// initialize from state persisted before a crash.
	rf.readPersist(persister.ReadRaftState())

	// start ticker goroutine to start elections.
	go rf.ticker()

	return rf
}
