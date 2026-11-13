# Mocking with moq

Read this when you write a unit test that needs a mock, or change an interface that has
one. This is the full text of SKILL.md → *Mocking with moq*.

Generate mocks with [matryer/moq](https://github.com/matryer/moq) via `go run` (no global install):

```
service/
  core.go
  tx-status_test.go          # unit tests next to the code
  mocks/
    order-repository.go      # one mock per file, interface name without -mock suffix
```

```makefile
MOQ_RUN:=go run github.com/matryer/moq@latest

.PHONY:gen-mock
gen-mock:
	$(MOQ_RUN) -out service/mocks/order-repository.go -pkg mocks ./service IOrderRepository
```

```go
package service_test

func TestService_Method(t *testing.T) {
    orderRepo := &mocks.IOrderRepositoryMock{
        FindByIDFunc: func(ctx context.Context, id domain.OrderID) (*domain.Order, error) {
            return &domain.Order{ID: id}, nil
        },
    }
    svc := service.NewCore(orderRepo, nil, nil)
    // ...
}
```

- Mocks live in `<package>/mocks/`, package `mocks`.
- Tests use `package <name>_test` (black-box).
- Regenerate with `make gen-mock` after interface changes.
