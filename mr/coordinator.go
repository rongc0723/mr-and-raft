package mr

import (
	"log"
	"net"
	"net/http"
	"net/rpc"
	"os"
	"sync"
	"time"
)

type Coordinator struct {
	// Your definitions here.
	mapTasks    []*Task
	reduceTasks []*Task
	mapsLeft    int
	reducesLeft int
	nReduce     int
	mu          sync.Mutex
}

type Task struct {
	num      int
	status   string
	filename string
}

// Your code here -- RPC handlers for the worker to call.

// assign a woker a task
func (c *Coordinator) GetTask(args *GetTaskArgs, reply *GetTaskReply) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.mapsLeft != 0 {
		for i, task := range c.mapTasks {
			if task.status == "unstarted" {
				c.mapTasks[i].status = "in progress"
				reply.FileName = task.filename
				reply.TaskType = "map"
				reply.NReduce = c.nReduce
				reply.Id = task.num
				go c.DetectStraggler(task.num, "map")
				break
			}
		}
	} else {
		if c.reducesLeft == 0 {
			reply.Done = true
			return nil
		}
		for i, task := range c.reduceTasks {
			if task.status == "unstarted" {
				c.reduceTasks[i].status = "in progress"
				reply.TaskType = "reduce"
				reply.Id = task.num
				reply.NMap = len(c.mapTasks)
				go c.DetectStraggler(task.num, "reduce")
				break
			}
		}
	}
	return nil
}

func (c *Coordinator) DetectStraggler(idx int, taskType string) {
	time.Sleep(10 * time.Second)
	c.mu.Lock()
	defer c.mu.Unlock()
	if taskType == "map" && c.mapTasks[idx].status == "in progress" {
		c.mapTasks[idx].status = "unstarted"
	} else if taskType == "reduce" && c.reduceTasks[idx].status == "in progress" {
		c.reduceTasks[idx].status = "unstarted"
	}
}

func (c *Coordinator) MarkComplete(args *MarkCompleteArgs, reply *MarkCompleteReply) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if args.TaskType == "map" && c.mapTasks[args.Id].status == "in progress" {
		c.mapTasks[args.Id].status = "completed"
		c.mapsLeft--
		// fmt.Println("maps left", c.mapsLeft)
	} else if args.TaskType == "reduce" && c.reduceTasks[args.Id].status == "in progress" {
		c.reduceTasks[args.Id].status = "completed"
		c.reducesLeft--
	}
	return nil
}

// mr-main/mrcoordinator.go calls Done() periodically to find out
// if the entire job has finished.
func (c *Coordinator) Done() bool {
	ret := false
	c.mu.Lock()
	defer c.mu.Unlock()
	// Your code here.
	if c.reducesLeft == 0 {
		// fmt.Println("job is done")
		ret = true
	}
	return ret
}

// create a Coordinator.
// mr-main/mrcoordinator.go calls this function.
// nReduce is the number of reduce tasks to use.
func MakeCoordinator(files []string, nReduce int) *Coordinator {
	c := Coordinator{}
	c.mapsLeft = len(files)
	c.reducesLeft = nReduce
	c.nReduce = nReduce
	// fmt.Println("maps left", c.mapsLeft)
	// fmt.Println("reduces left", c.reducesLeft)

	// Your code here.
	c.mapTasks = make([]*Task, 0)
	for i, file := range files {
		c.mapTasks = append(c.mapTasks, &Task{i, "unstarted", file})
	}
	// fmt.Println("tasks", c.mapTasks)

	c.reduceTasks = make([]*Task, 0)
	for i := 0; i < nReduce; i++ {
		c.reduceTasks = append(c.reduceTasks, &Task{i, "unstarted", ""})
	}
	// fmt.Println("tasks", c.reduceTasks)

	c.server()
	return &c
}

// start a thread that listens for RPCs from worker.go
// DO NOT MODIFY
func (c *Coordinator) server() {
	rpc.Register(c)
	rpc.HandleHTTP()
	//l, e := net.Listen("tcp", ":1234")
	sockname := coordinatorSock()
	os.Remove(sockname)
	l, e := net.Listen("unix", sockname)
	if e != nil {
		log.Fatal("listen error:", e)
	}
	go http.Serve(l, nil)
}
