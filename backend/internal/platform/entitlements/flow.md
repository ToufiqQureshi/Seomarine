# Plan entitlement interface: flow

> Internal guide for developers and coding agents.

## Current behavior

The interface defines how a feature can request an organization entitlement check. In this repository, production code has no call sites; the only implementation is the test/migration `AllowAll` stub, which allows requests unless the context is canceled. Do not treat this package as active production plan enforcement.

## Dependencies

Feature services and plan/billing integration.

## Relevant code

- `entitlements.go`
- `entitlements_test.go`
