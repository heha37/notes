package mr

import (
	"log"
	"reflect"
	"strconv"
	"strings"
	"sync"
	"time"
)
import "net"
import "os"
import "net/rpc"
import "net/http"

const (
	TaskTypeMap int = iota
	TaskTypeReduce
)

const (
	TaskStatusIdle string = "IDLE"
	TaskStatusInProgress string = "IN_PROGRESS"
	TaskStatusCompleted string = "COMPLETED"
)

type WorkerInfo struct {
	ID int
}

type Task struct {
	ID int
	Type int
	Status string
	Worker *WorkerInfo
}

type Master struct {
	// Your definitions here.
	tasks []*Task
	idleTasks []int
	inProgressTasks []int
	completedMapTasks []int
	mapTaskNum int
	completedReduceTasks []int
	reduceTaskNum int
	mapTaskFile map[int]string
	reduceTaskFiles map[int][]string
	mapWorkerTask map[int]int
	workerKeepalive []int
	mapWorkerKeepalive map[int]time.Time

	mux sync.Mutex
}

func ArrayContain(array, elem interface{}) (ok bool, position int){
	ok = false
	vArray := reflect.ValueOf(array)
	for i := 0; i < vArray.Len(); i++ {
		if ObjectEqual(vArray.Index(i).Interface(), elem) {
			ok, position = true, i
			return
		}
	}
	return
}

func ObjectEqual(a, b interface{}) bool {
	if a == nil || b == nil {
		return a == b
	}
	return reflect.DeepEqual(a, b)
}

// Your code here -- RPC handlers for the worker to call.

//
// an example RPC handler.
//
// the RPC argument and reply types are defined in rpc.go.
//
func (m *Master) Example(args *ExampleArgs, reply *ExampleReply) error {
	reply.Y = args.X + 1
	return nil
}

func (m *Master) RigsterWorker(args *RegisterWorkerArgs, reply *RegisterWorkerReply) error {
	m.mux.Lock()
	if _, ok := m.mapWorkerTask[args.WorkerID]; !ok {
		m.mapWorkerTask[args.WorkerID] = -1
		reply.Success = true
		reply.ErrMessage = ""
	} else {
		reply.ErrMessage = "worker exists"
	}

	m.workerKeepalive = append(m.workerKeepalive, args.WorkerID)
	m.mapWorkerKeepalive[args.WorkerID] = time.Now()

	m.mux.Unlock()
	return nil
}

func (m *Master) AssignTask(args *AskForTaskArgs, reply *AskForTaskReply) error {
	workerID := args.WorkerID
	reply.NoTask = false

	m.mux.Lock()
	if len(m.idleTasks) == 0 {
		reply.NoTask = true
		m.mux.Unlock()
		return nil
	}
	if workingTaskID, ok := m.mapWorkerTask[workerID]; !ok {
		reply.ErrMessage = "worker not registered"
		m.mux.Unlock()
		return nil
	} else {
		if workingTaskID != -1 {
			reply.ErrMessage = "there is already a task on this worker"
			m.mux.Unlock()
			return nil
		}
	}
	assignTaskID := m.idleTasks[0]
	assignTask := m.tasks[assignTaskID]
	m.idleTasks = m.idleTasks[1:]
	m.inProgressTasks = append(m.inProgressTasks, assignTaskID)
	m.mapWorkerTask[workerID] = assignTaskID
	m.tasks[assignTaskID].Status = TaskStatusInProgress

	reply.TaskID = assignTaskID
	switch assignTask.Type {
	case TaskTypeMap:
		reply.TaskType = "MAP"
		reply.InputFilePath = m.mapTaskFile[assignTaskID]
		reply.NReduce = m.reduceTaskNum
	case TaskTypeReduce:
		reply.TaskType = "REDUCE"
	}

	m.mapWorkerKeepalive[workerID] = time.Now()

	m.mux.Unlock()
	return nil
}

func (m *Master) AssignReduceTaskInputFiles(args *AskForReduceTaskInputFilesArg, reply *AskForReduceTaskInputFilesReply) error {
	taskID := args.TaskID

	m.mux.Lock()
	if _, ok := m.mapWorkerTask[args.WorkerID]; !ok {
		reply.ErrMessage = "worker not registered"
		m.mux.Unlock()
		return nil
	}
	if m.mapTaskNum != len(m.completedMapTasks) {
		reply.MapTasksNotDone = true
		m.mux.Unlock()
		return nil
	}
	ok, _ := ArrayContain(m.completedReduceTasks, taskID)
	if ok {
		reply.TaskIsCompleted = true
		m.mux.Unlock()
		return nil
	}

	reply.InputFiles = m.reduceTaskFiles[taskID-m.mapTaskNum]
	reply.MapNum = m.mapTaskNum

	m.mapWorkerKeepalive[args.WorkerID] = time.Now()

	m.mux.Unlock()
	return nil
}

func (m *Master) CompleteTask(args *CompleteTaskArgs, reply *CompleteTaskReply) error {
	workerID := args.WorkerID
	taskID := args.TaskId

	m.mux.Lock()
	if _, ok := m.mapWorkerTask[workerID]; !ok {
		reply.ErrMessage = "worker not registered"
		m.mux.Unlock()
		return nil
	}
	switch args.Type {
	case "MAP":
		ok, pos := ArrayContain(m.inProgressTasks, taskID)
		if ok {
			copy(m.inProgressTasks[pos:], m.inProgressTasks[pos+1:])
			m.inProgressTasks[len(m.inProgressTasks)-1] = -1
			m.inProgressTasks = m.inProgressTasks[:len(m.inProgressTasks)-1]
			m.completedMapTasks = append(m.completedMapTasks, taskID)
			m.tasks[taskID].Status = TaskStatusCompleted
			for _, iFile := range args.IntermediateFiles {
				fileArgs := strings.Split(iFile, "-")
				fileID, err := strconv.Atoi(fileArgs[2])
				if err != nil {
					m.mux.Unlock()
					return err
				}
				m.reduceTaskFiles[fileID] = append(m.reduceTaskFiles[fileID], iFile)
			}
			m.mapWorkerTask[workerID] = -1
			reply.Success = true
		} else {
			reply.Success = false
			reply.ErrMessage = "The task is not in the inprogress queue"
			m.mux.Unlock()
			return nil
		}
	case "REDUCE":
		ok, pos := ArrayContain(m.inProgressTasks, taskID)
		if ok {
			copy(m.inProgressTasks[pos:], m.inProgressTasks[pos+1:])
			m.inProgressTasks[len(m.inProgressTasks)-1] = -1
			m.inProgressTasks = m.inProgressTasks[:len(m.inProgressTasks)-1]
			m.completedReduceTasks = append(m.completedReduceTasks, taskID)
			m.tasks[taskID].Status = TaskStatusCompleted

			m.mapWorkerTask[workerID] = -1
			reply.Success = true
		} else {
			reply.Success = false
			reply.ErrMessage = "The task is not in the inprogress queue"
			m.mux.Unlock()
			return nil
		}
	}

	m.mapWorkerKeepalive[workerID] = time.Now()

	m.mux.Unlock()
	return nil
}

//
// start a thread that listens for RPCs from worker.go
//
func (m *Master) server() {
	rpc.Register(m)
	rpc.HandleHTTP()
	//l, e := net.Listen("tcp", ":1234")
	sockname := masterSock()
	os.Remove(sockname)
	l, e := net.Listen("unix", sockname)
	if e != nil {
		log.Fatal("listen error:", e)
	}
	go http.Serve(l, nil)
}

func (m *Master) killWorkers(queue []int) {
	for _, workerID := range queue {
		ok, pos := ArrayContain(m.workerKeepalive, workerID)
		if ok {
			taskID := m.mapWorkerTask[workerID]
			m.idleTasks = append(m.idleTasks, taskID)
			m.tasks[taskID].Status = TaskStatusIdle

			copy(m.inProgressTasks[pos:], m.inProgressTasks[pos+1:])
			m.inProgressTasks[len(m.inProgressTasks)-1] = -1
			m.inProgressTasks = m.inProgressTasks[:len(m.inProgressTasks)-1]

			delete(m.mapWorkerTask, workerID)

			m.workerKeepalive[pos] = m.workerKeepalive[len(m.workerKeepalive)-1]
			m.workerKeepalive[len(m.workerKeepalive)-1] = -1
			m.workerKeepalive = m.workerKeepalive[:len(m.workerKeepalive)-1]
			delete(m.mapWorkerKeepalive, workerID)
		}

	}
}

func (m *Master) keepaliveFunc() {
	deletedQueue := []int{}
	for _, wk := range m.workerKeepalive {
		wkTime := m.mapWorkerKeepalive[wk]
		now := time.Now()
		if now.Sub(wkTime) >= time.Duration(10) * time.Second {
			deletedQueue = append(deletedQueue, wk)
		}
	}
	m.killWorkers(deletedQueue)
}

func (m *Master) keepalive() {
	go func() {
		interval := time.Duration(5) * time.Second
		ticker := time.NewTicker(interval)
		for {
			select {
			case <-ticker.C:
				m.keepaliveFunc()
			}
		}
	}()
}

//
// main/mrmaster.go calls Done() periodically to find out
// if the entire job has finished.
//
func (m *Master) Done() bool {
	ret := false

	// Your code here.
	if len(m.idleTasks) == 0 && len(m.inProgressTasks) == 0 {
		ret = true
	}

	return ret
}

func generateTasks(files []string, nReduce int)	(tasks []*Task, mapTaskFile map[int]string) {
	nMap := len(files)
	taskNum := nMap + nReduce
	tasks = make([]*Task, taskNum)
	mapTaskFile = make(map[int]string)
	for i, file := range files {
		mapTask := &Task{
			ID: i,
			Status: TaskStatusIdle,
			Type: TaskTypeMap,
		}
		tasks[i] = mapTask
		mapTaskFile[i] = file
	}
	for i := 0; i < nReduce; i++ {
		reduceTask := &Task{
			ID: nMap + i,
			Status: TaskStatusIdle,
			Type: TaskTypeReduce,
		}
		tasks[nMap+i] = reduceTask
	}
	return
}

//
// create a Master.
// main/mrmaster.go calls this function.
// nReduce is the number of reduce tasks to use.
//
func MakeMaster(files []string, nReduce int) *Master {
	m := Master{}

	// Your code here.
	m.mux.Lock()
	m.tasks, m.mapTaskFile = generateTasks(files, nReduce)
	m.idleTasks = make([]int, 0)
	m.completedMapTasks = make([]int, 0)
	m.completedReduceTasks = make([]int, 0)
	m.workerKeepalive = make([]int, 0)
	m.mapWorkerTask = make(map[int]int)
	m.reduceTaskFiles = make(map[int][]string)
	m.mapWorkerKeepalive = make(map[int]time.Time)
	for i:=0; i<nReduce; i++ {
		m.reduceTaskFiles[i] = make([]string, 0)
	}
	for _, task := range m.tasks {
		m.idleTasks = append(m.idleTasks, task.ID)
	}
	m.mapTaskNum = len(files)
	m.reduceTaskNum = nReduce
	m.mux.Unlock()

	m.server()
	return &m
}
