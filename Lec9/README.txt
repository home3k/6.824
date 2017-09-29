6.824 2017 Lecture 9: Distributed Transactions

Topics:
  distributed transactions = distributed commit + concurrency control
  two-phase commit

What's a transaction?
  suppose an application wants to make complex updates to a DB
    how to cope with crashes and concurrency?
    there are many ways to apprach this problem
    transactions are the gold standard of convenience, though expensive
  example -- new account creation:
    begin_transaction()
    if "alice" not in password table:
      add alice to password table
      add alice to profile table
    commit_transaction()
  transactions must be:
    atomic: all writes occur, or none, even if failures
    serializable: final result is as if transactions executed one by one
    durable: committed writes survive crash and restart
  these are the "ACID" properties
  applications rely on these properties!
  we are interested in *distributed* transactions

Distributed commit:
  A bunch of computers are cooperating on some task
  Each computer has a different role
  Want to ensure atomicity: all execute, or none execute
  Challenges: failures, performance

Example:
  calendar system, each user has a calendar
  want to schedule meetings with multiple participants
  one server holds calendars of users A-M, another server holds N-Z
  [diagram: client, two servers]
  sched(u1, u2, t):
    begin_transaction()
    ok1 = reserve(u1, t)
    ok2 = reserve(u2, t)
    if ok1 and ok2:
      if commit_transaction():
        print "yes"
    else
      abort_transaction()
  the reserve() calls are RPCs to the two calendar servers
  We want atomicity: both reserve, or neither reserves.
  What if 1st reserve() returns true, and then:
    2nd reserve() returns false (time not available, or u2 doesn't exist)
    2nd reserve() doesn't return (lost RPC msg, or u2's server crashes)
    client fails before 2nd reserve()
  We need a "distributed commit protocol"

Idea: tentative changes, later commit or undo (abort)
  reserve_handler(u, t):
    if u[t] is free:
      temp_u[t] = taken -- A TEMPORARY VERSION
      return true
    else:
      return false
  commit_handler():
    copy temp_u[t] to real u[t]
  abort_handler():
    discard temp_u[t]

Idea: single entity decides whether to commit
  to ensure agreement
  let's call it the Transaction Coordinator (TC)
  [time diagram: client, TC, A, B]
  client sends RPCs to A, B
  client's commit_transaction() sends "go" to TC
  TC/A/B execute distributed commit protocol...
  TC reports "commit" or "abort" to client

We want two properties for distributed commit protocol:
  Correctness:
    if any commit, none abort
    if any abort, none commit
  Performance:
    (since doing nothing is correct...)
    if no failures, and all can commit, then commit.
    if failures, come to some conclusion ASAP.

We're going to develop a protocol called "two-phase commit"
  Used by distributed databases for multi-server transactions
  A common pattern in many distributed systems

Two-phase commit without failures:
  [time diagram: client, TC, A, B]
  client sends reserve() RPCs to A, B
  client sends "go" to TC
  TC sends "prepare" messages to A and B.
  A and B respond, saying whether they're willing to commit.
    Respond "yes" if haven't crashed, timed out, &c.
  If both say "yes", TC sends "commit" messages.
  If either says "no", TC sends "abort" messages.
  A/B commit if they get a commit message.
    I.e. they write temp_* to the real DB.

Why is this correct so far?
  Neither can commit unless they both agreed.
  Crucial that neither changes mind after responding to prepare
    Not even if failure!

What about failures?
  Network broken/lossy/slow
  Server crashes
  What is our goal w.r.t. failure?
    Resume correct operation after repair
    I.e. recovery, *not* availability (since no replication here)
  Single symptom: timeout when expecting a message.

Where do hosts wait for messages?
  1) TC waits for yes/no.
  2) A and B wait for prepare and commit/abort.

Termination protocol summary:
  TC t/o for yes/no -> abort
  B t/o for prepare, -> abort
  B t/o for commit/abort, B voted no -> abort
  B t/o for commit/abort, B voted yes -> block

TC timeout while waiting for yes/no from A/B.
  TC has not sent any "commit" messages.
  So TC can safely abort, and send "abort" messages.

A/B timeout while waiting for prepare from TC
  have not yet responded to prepare, so TC can't have decided commit
  so A/B can unilaterally abort
  respond "no" to future prepare

A/B timeout while waiting for commit/abort from TC.
  Let's talk about just B (A is symmetric).
  If B voted "no", it can unilaterally abort.
  So what if B voted "yes"?
  Can B unilaterally decide to abort?
    No! TC might have gotten "yes" from both,
    and sent out "commit" to A, but crashed before sending to B.
    So then A would commit and B would abort: incorrect.
  B can't unilaterally commit, either:
    A might have voted "no".

So: if B voted "yes", it must "block": wait for TC decision.

What if B crashes and restarts?
  If B sent "yes" before crash, B must remember!
  Can't change to "no" (and thus abort) after restart
  Since TC may have seen previous yes and told A to commit

Thus subordinates must write persistent (on-disk) state:
  B must remember on disk before saying "yes", including modified data.
  If B reboots, disk says "yes" but no "commit", B must ask TC.
    (this is The Question)
  If TC says "commit", B copies modified data to real data.

What if TC crashes and restarts?
  If TC might have sent "commit" or "abort" before crash, TC must remember!
    And repeat that if anyone asks (i.e. if A/B/client didn't get msg).
    Thus TC must write "commit" to disk before sending commit msgs.
  TC can't change its mind since A/B/client may have already acted.

This protocol is "two-phase commit".
  * All hosts that decide reach the same decision.
  * No commit unless everyone says "yes".
  * TC failure can make servers block until repair.

What about concurrent transactions?
  x and y are bank balances
  x and y start out as $10
  T1 is doing a transfer of $1 from x to y
  T1:
    add(x, 1)  -- server A
    add(y, -1) -- server B
  T2:
    tmp1 = get(x)
    tmp2 = get(y)
    print tmp1, tmp2

Problem:
  what if T2 runs between the two add() RPCs?
  then T2 will print 11, 10
  money will have been created!
  T2 should print 10,10 or 9,11

The traditional correctness definition is "serializability"
  results should be as if transactions ran one at a time in some order
  as if T1, then T2; or T2, then T1
    the results for the two differ; either is OK

You can test whether a specific execution is serializable by
  looking for a serial order that yields the same results.
  there's no such order for 11,10, but there is for 10,10 and 9,11

Why is serializability good for programmers?
  it allows application code to ignore concurrency
  just write the transaction to take system from one legal state to another
  internally, the transaction can temporarily violate invariants
    e.g. midway through T1
    but serializability guarantees other xactions won't notice

Why is serializability OK for performance?
  transactions that don't conflict can run in parallel
  since, if T3 and T4 don't conflict, *results* from T3 || T4
    will be the same as T3, then T4 (and T4, then T3)

How to decide if it's OK to let two transactions run in parallel?

"Two-phase locking" is one way to implement serializability
  each database record has a lock
  the lock is stored at the server that stores the record
  transaction must wait for and acquire a record's lock before using it
    thus add() handler implicitly acquires lock when it uses record x or y
  transaction holds its locks until *after* commit or abort

Why hold locks until after commit/abort?
  why not release as soon as done with the record?
  e.g. why not have T2 release x's lock after first get()?
    T1 could then execute between T2's get()s
    T2 would print 10,9
    but that is not a serializable execution: neither T1;T2 nor T2;T1

What are locks really doing?
  When transactions conflict, locks delay one to force serial execution.
  When transactions don't conflict, locks allow fast parallel execution.

How does locking interact with two-phase commit?
  Server must aquire and remember locks as it executes client requests.
    So client->server RPCS have two effects: acquire lock, use data.
  If server says "yes" to TC's prepare:
    Must remember locks and values across crash+restart!
    So must write locks+values to disk (in log), before replying "yes".
    If reboot, then COMMIT from TC, read locks+values from disk.
  If server has not said "yes" to a prepare:
    If crash+restart, server can release locks and discard new values.
      (or just forget about them during the crash)
    And then say "no" to TC's prepare message.

Today's paper: two-phase commit in R*
  R and R* were hugely influential early IBM DB research projects
  perhaps the first paper to explain details of two-phase commit
  for us, two topics:
    how to forget transactions
    speeding up read-only transactions

Forgetting in R*'s 2P protocol
  my description of 2PC assumes subordinates can ask TC at any time
    and that the TC remembers outcome of all transactions
    this is not practical -- servers must forget old transactions
  problem: what if subordinate sends a query about a forgotten xaction?
  solution in 2P:
    each subordinate replies with ACK to TC's COMMIT msg
    each subordinate can then forget
    TC waits for ACKs from all subordinates
    TC can then forget
  TC receives a query from subordinate about unknown transaction
    two cases can lead to TC not knowing abt a transaction:
      1. TC received ACKs from all subordinates, and then forgot
      2. TC crashed after sending prepares, before writing commit
    if #1, subordinate logged a COMMIT, so this can't happen
    if #2, TC could not have committed, so TC can reply ABORT
    ACK == subordinate promises not to ask about this xaction
  subordinate receives a COMMIT from TC abt unknown transaction
    (perhaps network lost its first ACK)
    subordinate must have prepared and committed, otherwise
      would still be merely prepared
    so subordinate can reply with ACK

Speeding up read-only transactions with R*'s PA (Presumed Abort)
  PA eliminates a round of messages for read-only transactions
    (this is one of many special cases you could streamline)
  I'll discuss completely read-only transactions
  they need to lock, and hold locks until they finish,
    to ensure serializability for r/o versus r/w transactions
  so TC needs a prepare phase to ask each if it is still alive
    and holding locks at the end of the transaction
  PA does this for read-only transactions:
    --prepare-->
    <--VOTE READ-- or <--VOTE NO--
  no second phase is needed!
    subordinates don't need to write anything
    subordinates can safely release locks on prepare
      since all reads have completed
  no log writes are needed!
  TC and subordinates can forget immediately!
  the reason:
    at all points it is safe to abort
    so neither TC nor subordinates ever need to send a query
    so TC and subordinates don't have to carefully preserve
      information across crash+reboot

2PC perspective
  Used in sharded DBs when a transaction uses data on multiple shards
  But it has a bad reputation:
    slow: multiple rounds of messages
    slow: disk writes
    locks are held over the prepare/commit exchanges; blocks other xactions
    TC crash can cause indefinite blocking, with locks held
  Thus usually used only in a single small domain
    E.g. not between banks, not between airlines, not over wide area
  Faster distributed transactions are an active research area:
    Lower message and persistence cost
    Special cases that can be handled with less work
    Wide-area transactions
    Less consistency, more burden on applications

Raft and two-phase commit solve different problems!
  Use Raft to get high availability by replicating
    i.e. to be able to operate when some servers are crashed
    the servers all do the *same* thing
  Use 2PC when each subordinate does something different
    And *all* of them must do their part
  2PC does not help availability
    since all servers must be up to get anything done
  Raft does not ensure that all servers do something
    since only a majority have to be alive

What if you want high availability *and* distributed commit?
  [diagram]
  Each "server" should be a Raft-replicated service
  And the TC should be Raft-replicated
  Run two-phase commit among the replicated services
  Then you can tolerate failures and still make progress
  You'll build something like this to transfer shards in Lab 4