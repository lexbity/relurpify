/*
Package platform provides infrastructure tools and services for Relurpify.

Architecture Principles:

 1. Dependency Direction: Platform is the foundation layer. Higher-level domains
    may import platform, but platform must not import those domains. This keeps
    platform independent from agent, execution, named-agent, and app code.

 2. Consumer Ports: Shared boundaries are owned by the consuming domain. Platform
    packages implement small interfaces declared by capability, governance,
    execution, or app packages instead of importing those domains directly.

 3. Tool Interface: Tools in platform implement ports.Tool. Execute and
    IsAvailable take only stdlib context.Context and explicit args — no
    execution envelope state parameter. Envelope state is handled by context and
    capability packages, not platform.

 4. Test Files: Test files may use higher-level test utilities when needed, but
    production code must adhere to strict layer boundaries.

Layering Rule:
  - platform/ must not import app/, named/, cognitionzoo/, execution/, context/,
    governance/, or capability/ packages in production code
  - higher-level domains may import platform packages only through approved
    composition points
  - Define local interfaces in the consuming package
  - Use dependency injection to receive higher-level callbacks
*/
package platform
