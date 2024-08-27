package mr

import (
	"encoding/json"
	"errors"
	"fmt"
	"hash/fnv"
	"io"
	"log"
	"net/rpc"
	"os"
	"sort"
	"strconv"
	"time"
)

// for sorting by key.
type ByKey []KeyValue

// for sorting by key.
func (a ByKey) Len() int           { return len(a) }
func (a ByKey) Swap(i, j int)      { a[i], a[j] = a[j], a[i] }
func (a ByKey) Less(i, j int) bool { return a[i].Key < a[j].Key }

// Map functions return a slice of KeyValue.
type KeyValue struct {
	Key   string
	Value string
}

// use ihash(key) % NReduce to choose the reduce
// task number for each KeyValue emitted by Map.
func ihash(key string) int {
	h := fnv.New32a()
	h.Write([]byte(key))
	return int(h.Sum32() & 0x7fffffff)
}

// mr-main/mrworker.go calls this function.
func Worker(mapf func(string, string) []KeyValue,
	reducef func(string, []string) string) {
	// Your worker implementation here.
	for {
		reply, err := GetTask()
		if err != nil {
			log.Fatal("GetTask failed")
		}
		if reply.Done {
			break
		}

		if reply.TaskType == "map" {
			err := DoMap(mapf, reply.FileName, reply.NReduce, reply.Id)
			if err != nil {
				log.Fatal("DoMap failed")
			}
			MarkComplete(reply.Id, "map")
			// fmt.Printf("worker %v is done mapping %v \n", reply.Id, reply.FileName)
		}
		if reply.TaskType == "reduce" {
			// fmt.Println("worker is waiting to reduce")
			DoReduce(reducef, reply.Id, reply.NMap)
			MarkComplete(reply.Id, "reduce")
		}
		time.Sleep(time.Second)
	}

}

// get task from coordinator
func GetTask() (*GetTaskReply, error) {
	args := GetTaskArgs{}
	reply := GetTaskReply{}

	ok := call("Coordinator.GetTask", &args, &reply)
	if ok {
		return &reply, nil
	}
	return nil, errors.New("GetTask failed")

}

func DoMap(mapf func(string, string) []KeyValue, FileName string, NReduce int, Id int) error {
	file, err := os.Open(FileName)
	if err != nil {
		log.Fatalf("cannot open %v", FileName)
	}

	content, err := io.ReadAll(file)
	if err != nil {
		log.Fatalf("cannot read %v", FileName)
	}

	file.Close()
	//generate key value pairs
	kva := mapf(FileName, string(content))

	//intialize the intermediate files
	for i := 0; i < NReduce; i++ {
		oname := "mr-" + strconv.Itoa(Id) + "-" + strconv.Itoa(i)
		_, err := os.Create(oname)
		if err != nil {
			log.Fatal("error: ", err)
			return err
		}
	}
	// fmt.Printf("map worker %v is mapping \n", Id)
	//iterate over kv pairs and write them to appropriate intermediate partitions
	for _, kv := range kva {
		partition := ihash(kv.Key) % NReduce
		fileName := "mr-" + strconv.Itoa(Id) + "-" + strconv.Itoa(partition)
		ofile, err := os.OpenFile(fileName, os.O_APPEND|os.O_WRONLY|os.O_CREATE, os.ModeAppend)
		if err != nil {
			log.Fatal("error opening file map ", fileName)
			return err
		}
		enc := json.NewEncoder(ofile)
		er := enc.Encode(&kv)
		if er != nil {
			log.Fatal("error writing to partition", er)
			return er
		}
		ofile.Close()
	}
	// fmt.Printf("map worker %v is done \n", Id)

	return nil

}

func DoReduce(reducef func(string, []string) string, Id int, NMap int) {
	// Your code here
	intermediate := []KeyValue{}
	for i := 0; i < NMap; i++ {
		fileName := "mr-" + strconv.Itoa(i) + "-" + strconv.Itoa(Id)
		file, err := os.Open(fileName)
		if err != nil {
			log.Fatal("error opening file reduce ", err)
		}
		dec := json.NewDecoder(file)
		for {
			var kv KeyValue
			if err := dec.Decode(&kv); err != nil {
				break
			}
			intermediate = append(intermediate, kv)
		}
		file.Close()
	}
	sort.Sort(ByKey(intermediate))

	oname := "mr-out-" + strconv.Itoa(Id)
	ofile, err := os.Create(oname)
	if err != nil {
		log.Fatal("error creating file", err)
	}
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

		fmt.Fprintf(ofile, "%v %v\n", intermediate[i].Key, output)

		i = j
	}
	ofile.Close()

}

func MarkComplete(Id int, taskType string) {
	args := MarkCompleteArgs{Id, taskType}
	reply := MarkCompleteReply{}
	ok := call("Coordinator.MarkComplete", &args, &reply)
	if !ok {
		log.Fatal("MarkComplete failed")
	}
}

// send an RPC request to the coordinator, wait for the response.
// usually returns true.
// returns false if something goes wrong.
// DO NOT MODIFY
func call(rpcname string, args interface{}, reply interface{}) bool {
	// c, err := rpc.DialHTTP("tcp", "127.0.0.1"+":1234")
	sockname := coordinatorSock()
	c, err := rpc.DialHTTP("unix", sockname)
	if err != nil {
		log.Fatal("dialing:", err)
	}
	defer c.Close()

	err = c.Call(rpcname, args, reply)
	if err == nil {
		return true
	}

	fmt.Println("Unable to Call", rpcname, "- Got error:", err)
	return false
}
