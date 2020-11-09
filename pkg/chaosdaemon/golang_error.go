package chaosdaemon

import (
	"context"
	"fmt"
	"github.com/chaos-mesh/chaos-mesh/pkg/bpm"
	"github.com/shirou/gopsutil/process"
	"os"
	"strings"

	pb "github.com/chaos-mesh/chaos-mesh/pkg/chaosdaemon/pb"
)

const(
	delve_tool_Bin = "/usr/local/bin/delve_tool"
)

func getErrorType(action pb.GolangErrorAction_Action)int {
	switch action {
	case pb.GolangErrorAction_SqlQueryErrorAction:
		return 0
	}
	return -1
}
//设置golang异常
func (s *daemonServer) SetGolangError(ctx context.Context, request *pb.GolangErrorRequest) (*pb.GolangErrorResponse, error) {
	log.Info("trying to set golang error to target container , ", "containerID", request.ContainerId , "Action" , request.Action)

	//错误类型
	errorType := getErrorType(request.Action.Action)
	//获取需要attach 的pid
	pid, err := s.crClient.GetPidFromContainerID(ctx, request.ContainerId)
	if err != nil {
		log.Error(err, "error while getting PID")
		return nil, err
	}
	//转化成秒
	duration := fmt.Sprintf("%ds", request.Duration)
	//delve server端口监听地址
	address := "127.0.0.1:30303"

	//运行delve tool 的参数
	args := fmt.Sprintf("--pid %v --address %s --type %d --duration %s", pid , address , errorType, duration)

	log.Info("executing" , "cmd" , delve_tool_Bin + " " + args)

	cmd := bpm.DefaultProcessBuilder(delve_tool_Bin , strings.Split(args , " ")...).
		EnableSuicide().
		SetIdentifier(request.ContainerId).
		Build()

	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	err = s.backgroundProcessManager.StartProcess(cmd)
	if err != nil {
		return nil, err
	}

	procState, err := process.NewProcess(int32(cmd.Process.Pid))
	if err != nil {
		return nil, err
	}
	ct, err := procState.CreateTime()
	if err != nil {
		if kerr := cmd.Process.Kill(); kerr != nil {
			log.Error(kerr, "kill delve tool failed", "request", request)
		}
		return nil, err
	}

	return &pb.GolangErrorResponse{
		StartTime: ct,
		Pid: pid,
	}, nil
}
