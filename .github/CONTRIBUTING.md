# Contributing to go-api-boot

We welcome contributions to go-api-boot! This document provides guidelines for contributing to the project.

## Quick Links
- [Project Homepage](https://github.com/SaiNageswarS/go-api-boot)
- [Documentation](https://pkg.go.dev/github.com/SaiNageswarS/go-api-boot)
- [Examples](https://github.com/SaiNageswarS/go-api-boot#examples)

## How to Contribute

### Reporting Issues
- Use GitHub Issues for bug reports and feature requests
- Search existing issues before creating new ones
- Include Go version, OS, and relevant code examples

### Development Setup
```bash
git clone https://github.com/SaiNageswarS/go-api-boot.git
cd go-api-boot
go mod download
go test ./...
```

### Pull Requests
1. Fork the repository
2. Create a feature branch
3. Add tests for new functionality
4. Ensure all tests pass: `go test ./...`
5. Run linting: `gofmt -s -w .`
6. Submit a pull request

## Code Standards
- Follow Go best practices and idioms
- Add comprehensive tests for new features
- Update documentation for API changes
- Maintain backward compatibility when possible

## Keywords and Focus Areas
This project focuses on:
- Go API framework
- Go dependency injection
- gRPC and HTTP server
- MongoDB ODM for Go
- Microservices in Go
- Go web framework
- Go cloud native development
- Temporal workers in Go
- Production-ready Go APIs

## Community
- GitHub Discussions for questions and ideas
- Issues for bug reports and feature requests
- Pull requests for contributions

Thank you for contributing to go-api-boot!
