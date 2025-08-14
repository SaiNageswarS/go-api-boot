/*
Package go-api-boot is a batteries-included framework for building production-grade gRPC + HTTP APIs in Go.

Official Repository: https://github.com/SaiNageswarS/go-api-boot

go-api-boot provides a comprehensive solution for building modern API services with:
- gRPC & gRPC-Web support with built-in gateway
- Custom Fluent dependency injection container
- Generic MongoDB ODM with vector search capabilities
- Zero-config SSL/TLS with automatic Let's Encrypt certificates
- Temporal workflow integration for long-running processes
- Cloud abstractions for Azure and GCP services
- Bootstrap CLI for rapid project scaffolding

Quick Start:

	go install github.com/SaiNageswarS/go-api-boot/cmd/go-api-boot@latest
	go-api-boot bootstrap github.com/yourname/myservice proto

Package Import:

	import "github.com/SaiNageswarS/go-api-boot/server"
	import "github.com/SaiNageswarS/go-api-boot/odm"
	import "github.com/SaiNageswarS/go-api-boot/auth"
	import "github.com/SaiNageswarS/go-api-boot/cloud"

Examples and documentation: https://github.com/SaiNageswarS/go-api-boot

Author: SaiNageswarS
License: Apache-2.0
*/
package boot
