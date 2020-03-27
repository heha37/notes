package mr

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"
	"os"
	"sort"
	"strings"
	"time"
)
import "log"
import "net/rpc"
import "hash/fnv"


//
// Map functions return a slice of KeyValue.
//
type KeyValue struct {
	Key   string
	Value string
}

// for sorting by key.
type ByKey []KeyValue

// for sorting by key.
func (a ByKey) Len() int           { return len(a) }
func (a ByKey) Swap(i, j int)      { a[i], a[j] = a[j], a[i] }
func (a ByKey) Less(i, j int) bool { return a[i].Key < a[j].Key }

//
// use ihash(key) % NReduce to choose the reduce
// task number for each KeyValue emitted by Map.
//
func ihash(key string) int {
	h := fnv.New32a()
	h.Write([]byte(key))
	return int(h.Sum32() & 0x7fffffff)
}

type fileAndEncoder struct {
	file *os.File
	encoder *json.Encoder
}

type IntermediateController struct {
	fileAndEncoders []*fileAndEncoder
	intermediateMap map[string]int
	OutputFiles []string
}

func (c *IntermediateController) Init() {
	c.fileAndEncoders = make([]*fileAndEncoder, 0)
	c.intermediateMap = make(map[string]int)
	c.OutputFiles = make([]string, 0)
}

func (c *IntermediateController) createFileAndEncoder(key string) (id int) {
	file, err := ioutil.TempFile("./", key+".tmp")
	if err != nil {
		log.Fatal(err)
	}
	encoder := json.NewEncoder(file)
	fae := &fileAndEncoder{
		file:    file,
		encoder: encoder,
	}

	c.fileAndEncoders = append(c.fileAndEncoders, fae)
	c.intermediateMap[key] = len(c.fileAndEncoders) - 1

	return len(c.fileAndEncoders) - 1
}

func (c *IntermediateController) Input(fileKey string , kv *KeyValue) {
	var id int
	var ok bool
	id, ok = c.intermediateMap[fileKey]
	if !ok {
		id = c.createFileAndEncoder(fileKey)
	}
	enc := c.fileAndEncoders[id].encoder
	err := enc.Encode(kv)
	if err != nil {
		log.Fatal(err)
	}
}

func (c *IntermediateController) Clear() {
	for _, fae := range c.fileAndEncoders {
		file := fae.file
		fileName := file.Name()
		newFileName := strings.Split(fileName, ".")[0]
		file.Close()
		err := os.Rename(fileName, newFileName)
		if err != nil {
			log.Fatal(err)
		}
		c.OutputFiles = append(c.OutputFiles, newFileName)
	}
}


//
// main/mrworker.go calls this function.
//
func Worker(mapf func(string, string) []KeyValue,
	reducef func(string, []string) string) {

	// Your worker implementation here.

	id := os.Getpid()
	success, err := CallRegisterWorker(id)
	if err != nil {
		log.Fatal(err)
		return
	}
	if !success {
		log.Fatal("register worker failed")
		return
	}

	var curTask *WorkerTask

	for {
		time.Sleep(time.Duration(1)*time.Second)
		var err error
		if curTask == nil {
			curTask, err = CallAskForTask(id)
			if err != nil {
				log.Print(err)
				continue
			}
		}

		switch curTask.Type {
		case "REDUCE":
			taskCompleted, mapTasksNotDone, mapNum, inputFiles, e:= CallAskForReduceTaskInputFiles(id, curTask.ID)
			if e != nil {
				log.Print(e)
			}
			if taskCompleted {
				curTask = nil
				continue
			}
			if mapTasksNotDone {
				continue
			}

			intermediate := []KeyValue{}
			for _, fileName := range inputFiles {
				file, e := os.Open(fileName)
				if e != nil {
					log.Print(e)
				}
				decoder := json.NewDecoder(file)
				for {
					var kv KeyValue
					if err := decoder.Decode(&kv); err != nil {
						break
					}
					intermediate = append(intermediate, kv)
				}
				file.Close()
			}
			sort.Sort(ByKey(intermediate))

			resultFile, _ := os.Create(fmt.Sprintf("mr-out-%d", curTask.ID-mapNum))
			i := 0
			for i < len(intermediate) {
				j := i + 1
				for j < len(intermediate) && intermediate[j].Key == intermediate[i].Key {
					j++
				}
				values := []string{}

				for k := i; k < j; k++ {
					values = append(values, intermediate[k].Value)
				}
				output := reducef(intermediate[i].Key, values)

				// this is the correct format for each line of Reduce output.
				fmt.Fprintf(resultFile, "%v %v\n", intermediate[i].Key, output)

				i = j
			}
			resultFile.Close()

			success, e := CallCompleteTask(id, curTask.ID, curTask.Type, []string{})
			if e != nil {
				log.Fatal(e)
				return
			}
			if !success {
				curTask = nil
				return
			}
			curTask = nil
		case "MAP":
			fileName := curTask.InputFilePath
			file, e := os.Open(fileName)
			if e != nil {
				log.Print(e)
			}
			data, e := ioutil.ReadAll(file)
			if e != nil {
				log.Print(e)
			}
			file.Close()
			controller := new(IntermediateController)
			controller.Init()

			kvs := mapf(fileName, string(data))
			filePrefix := fmt.Sprintf("mr-%d", curTask.ID)
			for _, kv := range kvs {
				intermedateFileName := fmt.Sprintf("%s-%d", filePrefix, ihash(kv.Key)%curTask.NReduce)
				controller.Input(intermedateFileName, &kv)
			}
			controller.Clear()

			success, e := CallCompleteTask(id, curTask.ID, curTask.Type, controller.OutputFiles)
			if e != nil {
				log.Fatal(e)
				return
			}
			if !success {
				curTask = nil
				return
			}
			curTask = nil
		}
	}

	// uncomment to send the Example RPC to the master.
	// CallExample()

}

//
// example function to show how to make an RPC call to the master.
//
// the RPC argument and reply types are defined in rpc.go.
//
func CallExample() {

	// declare an argument structure.
	args := ExampleArgs{}

	// fill in the argument(s).
	args.X = 99

	// declare a reply structure.
	reply := ExampleReply{}

	// send the RPC request, wait for the reply.
	call("Master.Example", &args, &reply)

	// reply.Y should be 100.
	fmt.Printf("reply.Y %v\n", reply.Y)
}

func CallRegisterWorker(id int) (success bool, err error){
	success = false
	args := RegisterWorkerArgs{}
	args.WorkerID = id

	reply := RegisterWorkerReply{}

	ok := call("Master.RigsterWorker", &args, &reply)
	if !ok {
		err = errors.New("calling register-worker failed")
		return
	}
	success = reply.Success
	if reply.ErrMessage != "" {
		err = fmt.Errorf("reply error: %s", reply.ErrMessage)
	}
	return
}

type WorkerTask struct {
	ID int
	Type string
	InputFilePath string
	NReduce int
}

func CallAskForTask(id int) (task *WorkerTask, err error){
	args := AskForTaskArgs{}
	args.WorkerID = id

	reply := AskForTaskReply{}
	ok := call("Master.AssignTask", &args, &reply)
	if !ok {
		err = errors.New("calling register-worker failed")
		return
	}

	if reply.ErrMessage != "" {
		err = fmt.Errorf("reply error: %s", reply.ErrMessage)
		return
	}

	if reply.NoTask {
		err = errors.New("no task")
		return
	}

	task = new(WorkerTask)
	task.ID = reply.TaskID
	task.Type = reply.TaskType
	task.InputFilePath = reply.InputFilePath
	task.NReduce = reply.NReduce

	return
}

func CallAskForReduceTaskInputFiles(workerID int, taskID int) (taskCompleted bool, mapTasksNotDone bool, mapNum int, intputFiles []string, err error) {
	args := AskForReduceTaskInputFilesArg{}
	args.WorkerID = workerID
	args.TaskID = taskID
	reply := AskForReduceTaskInputFilesReply{}
	ok := call("Master.AssignReduceTaskInputFiles", &args, &reply)
	if !ok {
		err = errors.New("calling ask-for-reduce-task-input-files failed")
		return
	}

	taskCompleted  = reply.TaskIsCompleted
	mapTasksNotDone = reply.MapTasksNotDone
	mapNum = reply.MapNum

	if !taskCompleted && !mapTasksNotDone {
		intputFiles = reply.InputFiles
	}
	return
}

func CallCompleteTask(workerID int, taskID int, taskType string, files []string) (success bool, err error) {
	success = false

	args := CompleteTaskArgs{}
	args.Type = taskType
	args.WorkerID = workerID
	args.TaskId = taskID
	if taskType == "MAP" {
		args.IntermediateFiles = files
	}
	reply := CompleteTaskReply{}
	ok := call("Master.CompleteTask", &args, &reply)
	if !ok {
		err = errors.New("calling complete-task failed")
		return
	}
	success = reply.Success
	if reply.ErrMessage != "" {
		err = fmt.Errorf("reply error: %s", reply.ErrMessage)
	}
	return
}

//
// send an RPC request to the master, wait for the response.
// usually returns true.
// returns false if something goes wrong.
//
func call(rpcname string, args interface{}, reply interface{}) bool {
	// c, err := rpc.DialHTTP("tcp", "127.0.0.1"+":1234")
	sockname := masterSock()
	c, err := rpc.DialHTTP("unix", sockname)
	if err != nil {
		log.Fatal("dialing:", err)
	}
	defer c.Close()

	err = c.Call(rpcname, args, reply)
	if err == nil {
		return true
	}

	fmt.Println(err)
	return false
}
