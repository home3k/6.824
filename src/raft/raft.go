package raft

//
// this is an outline of the API that raft must expose to
// the service (or tester). see comments below for
// each of these functions for more details.
//
// rf = Make(...)
//   create a new Raft server.
// rf.Start(command interface{}) (index, term, isleader)
//   start agreement on a new log entry
// rf.GetState() (term, isLeader)
//   ask a Raft for its current term, and whether it thinks it is leader
// ApplyMsg
//   each time a new entry is committed to the log, each Raft peer
//   should send an ApplyMsg to the service (or tester)
//   in the same server.
//

import "sync"
import (
	"labrpc"
	"math/rand"
	"time"
	"bytes"
	"encoding/gob"
	"fmt"
)

// import "bytes"
// import "encoding/gob"

const (
	Leader    = 1
	Candidate = 2
	Follower  = 3
)

const (
	start   = 1
	timeout = 2
)

//
// as each Raft peer becomes aware that successive log entries are
// committed, the peer should send an ApplyMsg to the service (or
// tester) on the same server, via the applyCh passed to Make().
//
type ApplyMsg struct {
	Index       int
	Command     interface{}
	UseSnapshot bool   // ignore for lab2; only used in lab3
	Snapshot    []byte // ignore for lab2; only used in lab3
}

//
// A Go object implementing a single Raft peer.
//
type Raft struct {
	mu        sync.Mutex          // Lock to protect shared access to this peer's state
	peers     []*labrpc.ClientEnd // RPC end points of all peers
	persister *Persister          // Object to hold this peer's persisted state
	me        int                 // this peer's index into peers[]

	// Your data here (2A, 2B, 2C).
	// Look at the paper's Figure 2 for a description of what
	// state a Raft server must maintain.
	commitIndex int
	lastApplied int

	nextIndex  []int
	matchIndex []int

	// persisted state
	currentTerm int
	votedFor    int
	logs        []LogEntry

	// role
	role int

	// timer
	electionTimerStatus  int
	heartbeatTimerStatus int
}

// return currentTerm and whether this server
// believes it is the leader.
func (rf *Raft) GetState() (int, bool) {

	var term int
	var isleader bool
	// Your code here (2A).
	term = rf.currentTerm
	if rf.role == Leader {
		isleader = true
	} else {
		isleader = false
	}
	return term, isleader
}

//
// save Raft's persistent state to stable storage,
// where it can later be retrieved after a crash and restart.
// see paper's Figure 2 for a description of what should be persistent.
//
func (rf *Raft) persist() {
	// Your code here (2C).
	// Example:
	// w := new(bytes.Buffer)
	// e := gob.NewEncoder(w)
	// e.Encode(rf.xxx)
	// e.Encode(rf.yyy)
	// data := w.Bytes()
	// rf.persister.SaveRaftState(data)
	w := new(bytes.Buffer)
	e := gob.NewEncoder(w)
	e.Encode(rf.currentTerm)
	e.Encode(rf.votedFor)
	e.Encode(rf.logs)
	data := w.Bytes()
	rf.persister.SaveRaftState(data)
}

//
// restore previously persisted state.
//
func (rf *Raft) readPersist(data []byte) {
	// Your code here (2C).
	// Example:
	// r := bytes.NewBuffer(data)
	// d := gob.NewDecoder(r)
	// d.Decode(&rf.xxx)
	// d.Decode(&rf.yyy)
	r := bytes.NewBuffer(data)
	d := gob.NewDecoder(r)
	d.Decode(&rf.currentTerm)
	d.Decode(&rf.votedFor)
	d.Decode(&rf.logs)

	if data == nil || len(data) < 1 { // bootstrap without any state?
		return
	}
}

//
// example RequestVote RPC arguments structure.
// field names must start with capital letters!
//
type RequestVoteArgs struct {
	// Your data here (2A, 2B).
	Term         int
	CandidateId  int
	LastLogIndex int
	LastLogTerm  int
}

//
// example RequestVote RPC reply structure.
// field names must start with capital letters!
//
type RequestVoteReply struct {
	// Your data here (2A).
	Term        int
	VoteGranted bool
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

type LogEntry struct {
	Term    int
	Content interface{}
}

//
// example RequestVote RPC handler.
//
func (rf *Raft) RequestVote(args *RequestVoteArgs, reply *RequestVoteReply) {
	rf.mu.Lock()
	fmt.Printf("server %d, receive vote from %d & votedfor is %d\n", rf.me, args.CandidateId, rf.votedFor)
	defer rf.mu.Unlock()
	reply.Term = rf.currentTerm
	if args.Term < rf.currentTerm {
		reply.VoteGranted = false
		return
	}
	if rf.votedFor == 0 || rf.votedFor == args.CandidateId {
		// todo  up-to-date log. check.
		reply.VoteGranted = true
		rf.votedFor = args.CandidateId
	} else {
		reply.VoteGranted = false
	}
}

// AppendEntries RPC handler
func (rf *Raft) AppendEntries(args *AppendEntriesArgs, reply *AppendEntriesReply) {

	fmt.Printf("server %d recv append entries\n", rf.me)

	rf.mu.Lock()
	defer rf.mu.Unlock()
	if args.Term >= rf.currentTerm {
		// convert to follower.
		rf.role = Follower
		rf.votedFor = 0
		rf.currentTerm = args.Term
		fmt.Printf("change role to %d\n", rf.role)
	}
	// restart timer
	rf.heartbeatTimerStatus = start

	reply.Term = rf.currentTerm
	if args.Term < rf.currentTerm {
		reply.Success = false
		return
	}
	if len(rf.logs)-1 < args.PrevLogIndex {
		reply.Success = false
		return
	}
	// todo conflict & append log

}

//
// example code to send a RequestVote RPC to a server.
// server is the index of the target server in rf.peers[].
// expects RPC arguments in args.
// fills in *reply with RPC reply, so caller should
// pass &reply.
// the types of the args and reply passed to Call() must be
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
// look at the comments in ../labrpc/labrpc.go for more details.
//
// if you're having trouble getting RPC to work, check that you've
// capitalized all field names in structs passed over RPC, and
// that the caller passes the address of the reply struct with &, not
// the struct itself.
//
func (rf *Raft) sendRequestVote(server int, args *RequestVoteArgs, reply *RequestVoteReply) bool {
	ok := rf.peers[server].Call("Raft.RequestVote", args, reply)
	return ok
}

func (rf *Raft) sendAppendEntries(server int, args *AppendEntriesArgs, reply *AppendEntriesReply) bool {
	ok := rf.peers[server].Call("Raft.AppendEntries", args, reply)
	return ok
}

//
// the service using Raft (e.g. a k/v server) wants to start
// agreement on the next command to be appended to Raft's log. if this
// server isn't the leader, returns false. otherwise start the
// agreement and return immediately. there is no guarantee that this
// command will ever be committed to the Raft log, since the leader
// may fail or lose an election.
//
// the first return value is the index that the command will appear at
// if it's ever committed. the second return value is the current
// term. the third return value is true if this server believes it is
// the leader.
//
func (rf *Raft) Start(command interface{}) (int, int, bool) {
	index := -1
	term := -1
	isLeader := true

	// Your code here (2B).

	return index, term, isLeader
}

//
// the tester calls Kill() when a Raft instance won't
// be needed again. you are not required to do anything
// in Kill(), but it might be convenient to (for example)
// turn off debug output from this instance.
//
func (rf *Raft) Kill() {
	// Your code here, if desired.
	fmt.Printf("server %d killed \n", rf.me)
}

//
// the service or tester wants to create a Raft server. the ports
// of all the Raft servers (including this one) are in peers[]. this
// server's port is peers[me]. all the servers' peers[] arrays
// have the same order. persister is a place for this server to
// save its persistent state, and also initially holds the most
// recent saved state, if any. applyCh is a channel on which the
// tester or service expects Raft to send ApplyMsg messages.
// Make() must return quickly, so it should start goroutines
// for any long-running work.
//
func Make(peers []*labrpc.ClientEnd, me int,
	persister *Persister, applyCh chan ApplyMsg) *Raft {
	rf := &Raft{}
	rf.peers = peers
	rf.persister = persister
	rf.me = me

	// Your initialization code here (2A, 2B, 2C).
	rf.role = Follower
	// initialize from state persisted before a crash
	rf.readPersist(persister.ReadRaftState())
	go regular(rf)

	return rf
}

func regular(rf *Raft) {
	fmt.Printf("server %d start regular\n", rf.me)
	rf.mu.Lock()
	rf.heartbeatTimerStatus = timeout
	rf.mu.Unlock()

	for {
		time.Sleep(time.Duration(heartbeatTimeout()) * time.Millisecond)
		if rf.heartbeatTimerStatus == start {
			rf.heartbeatTimerStatus = timeout
			continue
		}
		fmt.Printf("server %d regular timeout\n", rf.me)
		// timeout &
		// reset timer
		rf.heartbeatTimerStatus = timeout
		if rf.role == Follower || rf.role == Candidate {
			rf.role = Candidate
			rf.electionTimerStatus = timeout
			go func() {
				go doElection(rf)
				time.Sleep(time.Duration(electionTimeout()) * time.Millisecond)
				if rf.electionTimerStatus == timeout {
					if rf.role == Candidate {
						fmt.Printf("server %d election timeout role %d \n", rf.me, rf.role)
						// timeout
						go doElection(rf)
					}
				}

			}()
		} else {
			// leader
			doHeartbeat(rf)
		}

	}
}

func doElection(rf *Raft) {
	ok := election(rf)
	if ok {
		rf.mu.Lock()
		rf.role = Leader
		rf.mu.Unlock()
		doHeartbeat(rf)
	}
}

func election(rf *Raft) bool {

	rf.mu.Lock()
	peers := rf.peers
	me := rf.me
	rf.currentTerm++
	args, reply := new(RequestVoteArgs), new(RequestVoteReply)
	args.Term = rf.currentTerm
	args.CandidateId = rf.me
	args.LastLogIndex, args.LastLogTerm = rf.getLastEntry()
	rf.mu.Unlock()
	count := 0
	var wg sync.WaitGroup
	//fmt.Printf("server %d start election\n", rf.me)
	for server := range peers {
		if server == me {
			rf.mu.Lock()
			// vote for self.
			rf.votedFor = me
			rf.mu.Unlock()
			continue
		}
		wg.Add(1)
		go func(index int) {
			//fmt.Printf("server %d start election to server %d\n", rf.me, index)
			ok := rf.sendRequestVote(index, args, reply)
			if ok && reply.VoteGranted {
				//fmt.Printf("server %d start election to server %d & granted!!\n", rf.me, index)
				rf.mu.Lock()
				count++
				rf.mu.Unlock()
			} else {
				//fmt.Printf("server %d start election to server %d & NOT granted!!\n", rf.me, index)
			}
			wg.Done()
		}(server)
	}

	wg.Wait()

	successNodes := count + 1

	if successNodes > len(peers)/2 {
		fmt.Printf("server %d end election granted!!\n", rf.me)
		return true
	} else {
		fmt.Printf("server %d end election NOT granted!!\n", rf.me)
		return false
	}
}

func doHeartbeat(rf *Raft) {
	args, reply := new(AppendEntriesArgs), new(AppendEntriesReply)
	args.Term = rf.currentTerm
	args.LeaderId = rf.me
	args.LeaderCommit = rf.commitIndex
	args.PrevLogIndex, args.PrevLogTerm = rf.getLastEntry()
	for server := range rf.peers {
		if server == rf.me {
			continue
		}
		go func(index int) {
			fmt.Printf("server %d send append to server %d\n", rf.me, index)
			rf.sendAppendEntries(index, args, reply)
		}(server)
	}
}

func (rf *Raft) getLastEntry() (index int, term int) {
	term = 0
	index = len(rf.logs) - 1
	if index >= 0 {
		term = rf.logs[index].Term
	}
	return
}

func electionTimeout() (timeout int) {
	timeout = 400 + rand.Intn(400)
	return timeout
}

func heartbeatTimeout() (timeout int) {
	timeout = 100
	return
}
