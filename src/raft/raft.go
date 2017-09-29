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

// raft roles.
const (
	Leader    = 1
	Candidate = 2
	Follower  = 3
)

// timer status.
const (
	start   = 1
	timeout = 2
)

const voteForNone = -1

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

	// leaderId for redirect
	leaderId int

	// apply channel.
	applyCh chan ApplyMsg

	//temp for vote.
	voteGranted int
	voteNotGranted int

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
	Command interface{}
}

//
// example RequestVote RPC handler.
//
func (rf *Raft) RequestVote(args *RequestVoteArgs, reply *RequestVoteReply) {
	fmt.Printf("server %d, recv vote from %d & its votedfor is %d\n", rf.me, args.CandidateId, rf.votedFor)
	rf.mu.Lock()
	defer rf.mu.Unlock()
	if args.Term < rf.currentTerm {
		reply.VoteGranted = false
		return
	}
	if args.Term > rf.currentTerm {
		rf.initFollowerState(args.Term)
	}
	if rf.votedFor == 0 || rf.votedFor == args.CandidateId {
		// up-to-date log. check.
		lastIndex, lastTerm := rf.getLastEntry()
		if args.LastLogTerm < lastTerm || args.LastLogIndex < lastIndex {
			reply.VoteGranted = false
		} else {
			reply.VoteGranted = true
			rf.votedFor = args.CandidateId
		}
	} else {
		reply.VoteGranted = false
	}
}

// AppendEntries RPC handler
func (rf *Raft) AppendEntries(args *AppendEntriesArgs, reply *AppendEntriesReply) {

	fmt.Printf("server %d[%d] recv append entries from %d with term %d \n", rf.me, rf.currentTerm, args.LeaderId, args.Term)

	rf.mu.Lock()
	defer rf.mu.Unlock()
	if args.Term >= rf.currentTerm {
		rf.leaderId = args.LeaderId
		// ok, recv from leader, so convert to follower.
		rf.initFollowerState(args.Term)
	}
	// restart timer
	rf.heartbeatTimerStatus = start

	reply.Term = rf.currentTerm
	if args.Term < rf.currentTerm {
		reply.Success = false
		return
	}
	if len(rf.logs) < args.PrevLogIndex {
		reply.Success = false
		return
	}
	if args.PrevLogIndex > 0 && rf.logs[args.PrevLogIndex-1].Term != args.PrevLogTerm {
		// conflict, truncate the slice.
		rf.logs = rf.logs[0:args.PrevLogIndex]
		reply.Success = false
		return
	}

	//  It's ok, so append log

	for _, entry := range args.Entries {
		rf.logs = append(rf.logs, entry)
	}

	if args.LeaderCommit > rf.commitIndex {
		rf.commitIndex = args.LeaderCommit
		if len(rf.logs) < args.LeaderCommit {
			rf.commitIndex = len(rf.logs)
		}
	}
	reply.Success = true
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

	term = rf.currentTerm

	// Your code here (2B).
	if rf.role != Leader {
		isLeader = false
	}

	if !isLeader {
		return index, term, isLeader
	}

	fmt.Printf("raft start with %v \n", command)

	rf.doAppendEntries(command)

	rf.mu.Lock()
	defer rf.mu.Unlock()

	if rf.commitIndex > rf.lastApplied {
		fmt.Printf("begin to apply commit=%d apply=%d\n", rf.commitIndex, rf.lastApplied)
		rf.lastApplied++
		var msg = new(ApplyMsg)
		msg.Index = rf.lastApplied
		msg.Command = rf.logs[rf.lastApplied].Command
		index = rf.lastApplied
		rf.applyCh <- *msg
	}

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
	rf.votedFor = -1

	rf.applyCh = applyCh

	// Your initialization code here (2A, 2B, 2C).
	rf.role = Follower
	// initialize from state persisted before a crash
	rf.readPersist(persister.ReadRaftState())
	// start the goroutine.
	go rf.run()

	return rf
}

// raft run activity.
func (rf *Raft) run() {
	fmt.Printf("server %d start run\n", rf.me)
	rf.mu.Lock()
	rf.heartbeatTimerStatus = timeout
	rf.mu.Unlock()

	for {
		time.Sleep(time.Duration(electionTimeout()) * time.Millisecond)
		if rf.heartbeatTimerStatus == start {
			rf.heartbeatTimerStatus = timeout
			continue
		}
		// timeout & reset timer
		rf.heartbeatTimerStatus = timeout

		fmt.Printf("server %d[%d] run timeout\n", rf.me, rf.currentTerm)
		if rf.role == Follower || rf.role == Candidate {
			// convert to candidate & to do election.
			rf.role = Candidate
			rf.electionTimerStatus = timeout
			go func() {
				go rf.doElection()
				time.Sleep(time.Duration(electionTimeout()) * time.Millisecond)
				if rf.electionTimerStatus == timeout {
					// election timeout.
					if rf.role == Candidate {
						// still candidate? election retry.
						fmt.Printf("server %d election timeout role %d \n", rf.me, rf.role)
						// timeout
						go rf.doElection()
					}
				}

			}()
		} else {
			// leader. send heartbeat rpc.
			rf.doHeartbeat()
		}

	}
}

// do election
func (rf *Raft) doElection() {
	ok := rf.election()
	if ok {
		// ok election successful. reinitialized nextIndex & matchIndex
		rf.role = Leader
		rf.nextIndex = make([]int, len(rf.peers))
		rf.matchIndex = make([]int, len(rf.peers))
		for server := range rf.peers {
			rf.nextIndex[server] = len(rf.logs) + 1
			rf.matchIndex[server] = 0
		}
		rf.doHeartbeat()
	}
}

func (rf *Raft) election() bool {

	peers := rf.peers
	me := rf.me
	rf.currentTerm++
	rf.votedFor = -1
	args, reply := new(RequestVoteArgs), new(RequestVoteReply)
	args.Term = rf.currentTerm
	args.CandidateId = rf.me
	args.LastLogIndex, args.LastLogTerm = rf.getLastEntry()
	count := 0
	var wg sync.WaitGroup
	fmt.Printf("server %d start election\n", rf.me)
	for server := range peers {
		if server == me {
			// vote for self.
			rf.votedFor = me
			continue
		}
		wg.Add(1)
		go func(index int) {
			fmt.Printf("server %d start election to server %d\n", rf.me, index)
			ok := rf.sendRequestVote(index, args, reply)
			if ok && reply.VoteGranted {
				//fmt.Printf("server %d start election to server %d & granted!!\n", rf.me, index)
				count++
			} else {
				//fmt.Printf("server %d start election to server %d & NOT granted!!\n", rf.me, index)
				if reply.Term > rf.currentTerm {
					rf.initFollowerState(reply.Term)
				}
			}
			wg.Done()
		}(server)
	}

	wg.Wait()

	successNodes := count + 1

	// majority fo servers?
	if successNodes > len(peers)/2 {
		fmt.Printf("server %d end election granted!!\n", rf.me)
		return true
	} else {
		fmt.Printf("server %d end election NOT granted!!\n", rf.me)
		return false
	}
}

// append entries from client's command.
func (rf *Raft) doAppendEntries(command interface{}) {
	args, reply := new(AppendEntriesArgs), new(AppendEntriesReply)
	rf.mu.Lock()
	args.Term = rf.currentTerm
	args.LeaderId = rf.me
	args.LeaderCommit = rf.commitIndex
	args.PrevLogIndex, args.PrevLogTerm = rf.getLastEntry()
	logEntry := LogEntry{
		rf.currentTerm,
		command,
	}
	// append to local log.
	rf.logs = append(rf.logs, logEntry)
	rf.mu.Unlock()
	var wg sync.WaitGroup
	for server := range rf.peers {
		if server == rf.me {
			continue
		}
		wg.Add(1)
		go func(index int) {
			for {
				rf.mu.Lock()
				// logs
				if args.PrevLogIndex+1 >= rf.nextIndex[index] {
					args.Entries = rf.logs[rf.nextIndex[index]:]
				}
				rf.mu.Unlock()
				fmt.Printf("server %d send command append to server %d %v\n", rf.me, index, args)
				ok := rf.sendAppendEntries(index, args, reply)
				if ok {
					fmt.Printf("server %d append success from %d.\n", rf.me, index)
					// update followers nextIndex & matchIndex.
					rf.mu.Lock()
					rf.nextIndex[index] = len(rf.logs)
					rf.matchIndex[index] = rf.nextIndex[index] - 1
					rf.mu.Unlock()
					rf.doCommitCheck()
					wg.Done()
					// ok, break the loop.
					break
				} else {
					fmt.Printf("server %d append NOT success from %d.\n", rf.me, index)
					rf.mu.Lock()
					if reply.Term > rf.currentTerm {
						// change role.
						rf.initFollowerState(reply.Term)
						rf.mu.Unlock()
						break
					}
					// failed  decrement nextIndex.
					rf.nextIndex[index] = rf.nextIndex[index] - 1
					rf.mu.Unlock()
					// nothing to do, just retry.
				}
			}
		}(server)
	}

	wg.Wait()

	fmt.Println("wg Wait DONE.")
}

func (rf *Raft) doCommitCheck() {
	rf.mu.Lock()
	defer rf.mu.Unlock()
	index := len(rf.logs)
	radix := len(rf.peers) / 2

	fmt.Printf("server %d begin to do commitcheck WITH index %d \n", rf.me, index)

	for ; index > rf.commitIndex; index-- {
		majority := 0
		match := false
		for server := range rf.peers {
			if server == rf.me {
				majority++
			} else {
				if rf.matchIndex[server] >= index && rf.logs[index-1].Term == rf.currentTerm {
					majority++
				}
			}

			if majority > radix {
				match = true
			}

		}
		if match {
			break
		}
	}

	fmt.Printf("server %d finish to do commitcheck with %d \n", rf.me, index)

	// update commit index.
	if index > rf.commitIndex {
		rf.commitIndex = index
	}

}

// heartbeat.
func (rf *Raft) doHeartbeat() {
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
			//fmt.Printf("server %d send append to server %d\n", rf.me, index)
			ok := rf.sendAppendEntries(index, args, reply)
			if !ok {
				if reply.Term > rf.currentTerm {
					rf.currentTerm = reply.Term
					rf.role = Follower
					rf.votedFor = -1
				}
			}
		}(server)
	}
}

func (rf *Raft) getLastEntry() (index int, term int) {
	term = 0
	index = len(rf.logs)
	if index > 0 {
		term = rf.logs[index-1].Term
	}
	return
}

func (rf *Raft) initFollowerState(term int) {
	rf.currentTerm = term
	rf.role = Follower
	rf.votedFor = voteForNone
}

func (rf *Raft) quorum() (finished bool) {
	finished = false
	radix:=len(rf.peers)/2+1
	if rf.voteGranted >= radix {
		rf.role = Leader
		finished = true
		return
	}
	if rf.voteNotGranted >= radix {
		finished = true
		return
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
