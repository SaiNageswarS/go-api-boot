# go-api-boot

go-api-boot is a complete framework with batteries included to build API applications in Golang with database included. Upgraded to go 1.18 with support for generics making APIs more intuitive and easy to use.

# How is it Different

- Gives first hand support for grpc.
- Supports grpc-web out of box. No need of proxy etc.
- Supports odm for mongo.
- Adds JWT, logging middleware by default.
- Provides support for cloud (aws/azure) resources.
- APIs use generics of Go 1.18
- Provides handling secrets and config with Azure Keyvault integration and godotenv.
- Supports scheduled workers with monitoring and DB logging of running status of the workers.

Check https://github.com/Kotlang/authGo for example.

# Getting Started

Following environment variables are required for Go-API-Boot to start up.

```sh
MONGO_URI=mongodb://localhost:27017
ACCESS_SECRET=60ut694f-0a61-46f1-2175-8987b-24b56bd
```

## Starting grpc and web-proxy server.

```go
package main

import (
	pb "github.com/Kotlang/authGo/generated"
	"github.com/SaiNageswarS/go-api-boot/server"
)

var grpcPort = ":50051"
var webPort = ":8081"

func main() {
	// Load secrets from Keyvault and config through godotenv.
	server.LoadSecretsIntoEnv(true)
	inject := NewInject()

	corsConfig := cors.New(
		cors.Options{
			AllowedHeaders: []string{"*"},
		})
	bootServer := server.NewGoApiBoot(corsConfig)
	pb.RegisterLoginServer(bootServer.GrpcServer, inject.LoginService)
	pb.RegisterProfileServer(bootServer.GrpcServer, inject.ProfileService)

	bootServer.Start(grpcPort, webPort)
}
```

## JWT

By default authentication interceptor is added to all grpc calls. To skip jwt verification
add following method to service.

```
type LoginService struct {
	pb.UnimplementedLoginServer
}

//removing auth interceptor
func (u *LoginService) AuthFuncOverride(ctx context.Context, fullMethodName string) (context.Context, error) {
	return ctx, nil
}
```

## ODM

Using database requires creating a model and repository as below:

Model:

```
type ProfileModel struct {
	LoginId           string                 `bson:"_id" json:"loginId"`
	Name              string                 `bson:"name" json:"name"`
	PhotoUrl          string                 `bson:"photoUrl" json:"photoUrl"`
	Gender            string                 `bson:"gender" json:"gender"`
	IsVerified        bool                   `bson:"isVerified" json:"isVerified"`
	PreferredLanguage string                 `bson:"preferredLanguage" json:"preferredLanguage"`
	MetadataMap       map[string]interface{} `bson:"metadata"`
	CreatedOn         int64                  `bson:"createdOn" json:"createdOn"`
}

func (m *ProfileModel) Id() string {
	return m.LoginId
}
```

Repository:

```
type ProfileRepository struct {
	odm.AbstractRepository[models.ProfileModel]
}

func NewProfileRepo() *ProfileRepository {
	repo := odm.AbstractRepository[models.ProfileModel]{
		Database:       "auth",
		CollectionName: "profiles",
	}
	return &ProfileRepository{repo}
}
```

Usage:

```
// async - Returns channel of model and error.
// Can make multiple concurrent db calls.
profileRes, errorChan := profileRepo.FindOneById(userId)
select {
case profile := <- profileRes:
	logger.Info("Got profile of type profile model", zap.Info(profile))
case err := <- errorChan:
	logger.Error("Error fetching data", zap.Error(err))
}
```

## Cloud

* The framework provides support for both AWS, Azure and GCP out of box.
* Using cloud functions require setting required environment variables.
	* AWS_ACCESS_KEY_ID and AWS_SECRET_ACCESS_KEY in AWS.
	* AZURE_STORAGE_ACCOUNT and AZURE_STORAGE_ACCESS_KEY in Azure.

Usage:
```
var cloudFns cloud.Cloud = aws{}  // can be azure{} or gcp{} as well.
preSignedUrl, downloadUrl := cloudFns.GetPresignedUrl(s3Bucket, key, expiry)
```

## Boot Utils
Boot utils provide other grpc common utils used in API development.

### Streaming Util
For uploading files using client-side streaming, below API can be used to receive entire file data bytes. The API saves stream of bytes to an in-memory buffer and returns type of the stream.

Input Validations: 
- Acceptable MimeTypes: One can send list of acceptable mime-types as input parameter. This will be checked against header of the upload stream. If stream doesn't match acceptable mime-types, it will reject upload as soon as first 512 bytes are received. 
- MaxFileSize: Users can set max upload size limit. If stream size exceeds limit, upload will be rejected.

```
Proto:
rpc UploadProfileImage(stream UploadImageRequest) returns (UploadImageResponse) {}
message UploadImageRequest {
    bytes chunkData = 1;
}

Go:
imageData, mimeType, err := bootUtils.BufferGrpcServerStream([]string{"application/octet-stream"}, 2*1024*1024, func() ([]byte, error) {
		err := StreamContextError(stream.Context())
		if err != nil {
			return nil, err
		}

		req, err := stream.Recv()
		if err != nil {
			return nil, err
		}

		return req.ChunkData, nil
	})
```
