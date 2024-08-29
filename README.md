# go-api-boot
![Coverage](https://img.shields.io/badge/Coverage-15.2%25-red)
[![Go Report Card](https://goreportcard.com/badge/github.com/SaiNageswarS/go-api-boot)](https://goreportcard.com/report/github.com/SaiNageswarS/go-api-boot) [![Go Reference](https://pkg.go.dev/badge/github.com/SaiNageswarS/go-api-boot.svg)](https://pkg.go.dev/github.com/SaiNageswarS/go-api-boot)

go-api-boot is a complete framework with batteries included to build API applications in Golang with database included. Upgraded to go 1.18 with support for generics making APIs more intuitive and easy to use.

# How is it Different

- Gives first hand support for grpc.
- Supports grpc-web out of box. No need of proxy etc.
- Supports odm for mongo.
- Adds JWT, logging middleware by default.
- Provides support for cloud (aws/azure) resources.
- APIs use generics of Go 1.18
- Provides handling secrets and config with Azure Keyvault integration and .env.
- Supports scheduled workers with monitoring and DB logging of running status of the workers.
- Generates dependency injection wiring using github.com/google/wire. wire.go is generated as go-api-boot cli is used to create repositories/services. No need to hand code wire.go.

Check https://github.com/Kotlang/authGo for example.

# Getting Started

Following environment variables are required for Go-API-Boot to start up.

```sh
# Env variable name MONGO-URI is compatible with secret managers like keyvault.
MONGO-URI=mongodb://localhost:27017 
ACCESS-SECRET=60ut694f-0a61-46f1-2175-8987b-24b56bd
```

## Bootstrapping Project
Below commands will create a new go-api-boot project with Dependency Injection using wire, grpc server code and database repositories.

```sh
go install github.com/SaiNageswarS/go-api-boot/cmd/go-api-boot
go-api-boot bootstrap github.com/SaiNageswarS/quizGo/quizService proto
```

## Adding Database Repositories
Below command creates DB Model, Repository and adds the same to dependency injection.

```sh
go-api-boot repository Login
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

## Server

```
func main() {
	server.LoadEnv()

	app := InitializeApp()
	app.CloudFns.LoadSecretsIntoEnv()

	corsConfig := cors.New(
		cors.Options{
			AllowedHeaders: []string{"*"},
		})
	bootServer := server.NewGoApiBoot(
		server.WithCorsConfig(corsConfig),
		server.AppendUnaryInterceptors(app.UnaryInterceptors),
		server.AppendStreamInterceptors(app.StreamInterceptors))

	bootServer.Start(grpcPort, webPort)
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
	odm.UnimplementedBootRepository[ProfileModel]
}

func NewProfileRepo() *ProfileRepository {
	// baseRepo provides basic CRUD - Save, FindOneById, Find, DeleteById etc.
	baseRepo := odm.NewUnimplementedBootRepository[models.ProfileModel](
		odm.WithDatabase("authGo"),
		odm.WithCollectionName("profiles"),
	)
	return &ProfileRepository{baseRepo}
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

## Zero Config SSL Support
The go-api-boot framework provides seamless support for zero-configuration SSL/TLS using Let's Encrypt. This feature simplifies the process of securing your API with SSL, allowing you to enable HTTPS with minimal setup.

### Features
- **Automatic SSL Certificate Acquisition**: When SSL is enabled in the configuration, the framework automatically downloads and manages SSL certificates from Let's Encrypt.
- **ACME Challenge Handling**: The framework internally handles the ACME challenge process to prove ownership of the domain.
- **Minimal Configuration**: All you need to do is set the ssl configuration to true and provide your domain via an environment variable.

### How It Works
Just below two lines of code will enable SSL.

```go
os.Setenv("DOMAIN", "myservername.com")
bootServer := server.NewGoApiBoot(server.WithSSL(true))
```

### Deployment Instructions
- **Expose Port 80**: Ensure that port 80 is exposed when deploying your server. Let's Encrypt uses port 80 to complete the ACME challenge and verify domain ownership.
- **Use Port 443 for the Web Server**: It is recommended to run your web server on port 443, which is the standard port for HTTPS. However, user is free to pass any port to bootServer.Start()

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
