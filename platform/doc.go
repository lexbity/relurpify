/*
Package platform provides infrastructure tools and services for the agent framework.

Architecture Principles:

 1. Dependency Direction: Platform is the foundation layer. The framework MAY import
    platform, but platform MUST NOT import framework. This ensures the platform
    remains a stable foundation that framework builds upon.

 2. Contracts Package: The platform/contracts package defines interfaces and types
    that bridge framework and platform. Framework can re-export or wrap these
    types, but should not force platform to import framework types.

 3. Tool Interface: Tools in platform implement contracts.Tool. Execute and
    IsAvailable take only stdlib context.Context and explicit args — no
    contracts.Context state parameter. Framework-level envelope state
    (contextdata.Envelope) is handled in framework/, not platform/.

 4. Test Files: Test files in platform may import framework for test utilities,
    but production code must adhere to strict layer boundaries.

Layering Rule:
  - platform/ MUST NOT import framework/
  - framework/ MAY import platform/
  - Use platform/contracts for shared types
  - Define local interfaces in the consuming package
  - Use dependency injection to receive framework callbacks
*/
package platform
