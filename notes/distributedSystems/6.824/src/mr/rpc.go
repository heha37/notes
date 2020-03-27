package mr

//
// RPC definitions.
//
// remember to capitalize all names.
//

import "os"
import "strconv"

//
// example to show how to declare the arguments
// and reply for an RPC.
//

type ExampleArgs struct {
	X int
}

type ExampleReply struct {
	Y int
}

// Add your RPC definitions here.

type AskForTaskArgs struct {
	WorkerID int
}

type AskForTaskReply struct {
	TaskID int
	TaskType string
	InputFilePath string
	ReduceInputFiles []string
	NoTask bool
	NReduce int
	ErrMessage string
}

type CompleteTaskArgs struct {
	WorkerID int
	TaskId int
	Type string
	IntermediateFiles []string
}

type CompleteTaskReply struct {
	Success bool
	ErrMessage string
}

type RegisterWorkerArgs struct {
	WorkerID int
}

type RegisterWorkerReply struct {
	Success bool
	ErrMessage string
}

type AskForReduceTaskInputFilesArg struct {
	WorkerID int
	TaskID int
}

type AskForReduceTaskInputFilesReply struct {
	InputFiles []string
	MapNum int
	MapTasksNotDone bool
	TaskIsCompleted bool
	ErrMessage string
}

// Cook up a unique-ish UNIX-domain socket name
// in /var/tmp, for the master.
// Can't use the current directory since
// Athena AFS doesn't support UNIX-domain sockets.
func masterSock() string {
	s := "/var/tmp/824-mr-"
	s += strconv.Itoa(os.Getuid())
	return s
}
