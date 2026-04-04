 Two intentional non-implementations:
                                      
  - CommandQueryBuilder: The plan's capabilityRegistry interface doesn't map
  cleanly to binary names (tool names ≠ binary names). The current allow-all   
  empty list is the safe default. This can be revisited when the wiring point
  (manifest bash_permissions.executables → CommandLineTools) is clearer.       
  - normalizeMultilineJSONStringLiterals gating: The function is already
  effectively conditional — it only acts as a fallback when json.Unmarshal     
  fails, and for non-qwen models the JSON is valid so it's a no-op. Threading a
   profile flag through framework/capability/tool_formatting.go would add      
  complexity for no observable behavioral difference.