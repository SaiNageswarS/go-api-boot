package server

import (
	"fmt"
	"net/http"

	"github.com/SaiNageswarS/go-api-boot/logger"
	"go.uber.org/zap"
	"google.golang.org/grpc"
)

const grpcContentType = "application/grpc"
const grpcWebContentType = "application/grpc-web"
const grpcWebTextContentType = "application/grpc-web-text"

type WebProxy struct {
	handler       http.Handler
	endPointsFunc func() []string
}

func GetWebProxy(server *grpc.Server) WebProxy {
	endPointsFunc := func() []string {
		return listGRPCResources(server)
	}

	return WebProxy{
		handler:       server,
		endPointsFunc: endPointsFunc,
	}
}

func (w WebProxy) ServeHTTP(resp http.ResponseWriter, req *http.Request) {
	grpcReq, isTextFormat := interceptGrpcRequest(req)
	grpcResp := getWebProxyResponse(resp, isTextFormat)
	logger.Info("WebProxy.ServeHTTP: ", zap.String("Url", grpcReq.URL.Path))

	w.handler.ServeHTTP(grpcResp, grpcReq)
	grpcResp.finishRequest(grpcReq)
}

func listGRPCResources(server *grpc.Server) []string {
	ret := []string{}
	for serviceName, serviceInfo := range server.GetServiceInfo() {
		for _, methodInfo := range serviceInfo.Methods {
			fullResource := fmt.Sprintf("/%s/%s", serviceName, methodInfo.Name)
			ret = append(ret, fullResource)
		}
	}
	return ret
}
