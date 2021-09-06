# go-api-boot

go-api-boot is a complete framework with batteries included to build API applications in Golang with database included.

# How is it Different

- Gives first hand support for grpc.
- Supports grpc-web out of box. No need of proxy etc.
- Supports odm for mongo.
- Adds JWT, logging middleware by default.
- Provides support for cloud (aws/azure) resources.

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
	inject := NewInject()

	bootServer := server.NewGoApiBoot()
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
	odm.AbstractRepository
}

func NewProfileRepo() *ProfileRepository {
	repo := odm.AbstractRepository{
		Database:       "auth",
		CollectionName: "profiles",
		Model:          reflect.TypeOf(models.ProfileModel{}),
	}
	return &ProfileRepository{repo}
}
```

Usage:

```
// async - Returns channel
// Can make multiple concurrent db calls.
profileRes := <-profileRepo.FindOneById(userId)
profile := profileRes.Value.(*ProfileModel)
```
