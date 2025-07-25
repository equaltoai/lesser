#!/usr/bin/env python3
"""
Analyze stub implementations in the Lesser codebase and generate actionable recommendations.
"""

import os
import re
import json
from dataclasses import dataclass
from typing import List, Dict, Tuple
from collections import defaultdict
from datetime import datetime

@dataclass
class StubInstance:
    file_path: str
    line_number: int
    line_content: str
    stub_type: str
    severity: str
    function_name: str = None

class StubAnalyzer:
    def __init__(self, base_path="."):
        self.base_path = base_path
        self.stub_patterns = {
            "for_now_empty": {
                "pattern": r"//.*For now.*return.*empty",
                "severity": "HIGH",
                "description": "Explicitly returning empty data"
            },
            "would_normally": {
                "pattern": r"would normally",
                "severity": "HIGH", 
                "description": "Function not doing what it should"
            },
            "todo_implement": {
                "pattern": r"// TODO:.*[Ii]mplement",
                "severity": "MEDIUM",
                "description": "Implementation pending"
            },
            "not_implemented": {
                "pattern": r"not implemented|NotImplemented",
                "severity": "HIGH",
                "description": "Explicitly not implemented"
            },
            "panic_not_implemented": {
                "pattern": r"panic.*not implemented",
                "severity": "CRITICAL",
                "description": "Panics on use"
            },
            "placeholder": {
                "pattern": r"placeholder|dummy|stub",
                "severity": "MEDIUM",
                "description": "Placeholder implementation"
            },
            "empty_return": {
                "pattern": r"return \[\](map\[string\])?any{}, nil|return \[\]string{}, nil",
                "severity": "HIGH",
                "description": "Returns empty collection"
            }
        }
        
        self.critical_paths = [
            "cmd/api/handlers/",
            "pkg/storage/dynamodb/",
            "cmd/export-generator/",
            "graph/schema.resolvers.go"
        ]
        
    def find_stubs(self) -> List[StubInstance]:
        stubs = []
        
        for root, _, files in os.walk(self.base_path):
            for file in files:
                if file.endswith(('.go', '.ts', '.js')):
                    file_path = os.path.join(root, file)
                    if self._should_skip_file(file_path):
                        continue
                        
                    try:
                        with open(file_path, 'r', encoding='utf-8') as f:
                            content = f.readlines()
                            
                        for line_num, line in enumerate(content, 1):
                            for stub_type, config in self.stub_patterns.items():
                                if re.search(config["pattern"], line, re.IGNORECASE):
                                    func_name = self._extract_function_name(content, line_num - 1)
                                    stubs.append(StubInstance(
                                        file_path=file_path.replace(self.base_path + "/", ""),
                                        line_number=line_num,
                                        line_content=line.strip(),
                                        stub_type=stub_type,
                                        severity=config["severity"],
                                        function_name=func_name
                                    ))
                    except Exception as e:
                        print(f"Error reading {file_path}: {e}")
                        
        return stubs
    
    def _should_skip_file(self, file_path: str) -> bool:
        skip_dirs = ['test_venv', 'node_modules', '.git', '__pycache__', 'vendor']
        return any(skip_dir in file_path for skip_dir in skip_dirs)
    
    def _extract_function_name(self, content: List[str], line_index: int) -> str:
        """Extract the function name from the surrounding context."""
        # Look backwards for function definition
        for i in range(line_index, max(0, line_index - 10), -1):
            match = re.match(r'^func\s+(\([^)]+\)\s+)?(\w+)', content[i])
            if match:
                return match.group(2)
        return None
    
    def _is_critical_path(self, file_path: str) -> bool:
        return any(critical in file_path for critical in self.critical_paths)
    
    def analyze(self) -> Dict:
        stubs = self.find_stubs()
        
        # Group by file
        by_file = defaultdict(list)
        for stub in stubs:
            by_file[stub.file_path].append(stub)
        
        # Group by severity
        by_severity = defaultdict(list)
        for stub in stubs:
            by_severity[stub.severity].append(stub)
        
        # Find most problematic files
        problematic_files = sorted(
            [(path, len(stubs)) for path, stubs in by_file.items()],
            key=lambda x: x[1],
            reverse=True
        )[:10]
        
        # Critical functions (named functions with HIGH/CRITICAL severity)
        critical_functions = [
            stub for stub in stubs 
            if stub.severity in ['HIGH', 'CRITICAL'] and stub.function_name
        ]
        
        return {
            "total_stubs": len(stubs),
            "by_severity": {
                severity: len(stubs) for severity, stubs in by_severity.items()
            },
            "problematic_files": problematic_files,
            "critical_functions": critical_functions,
            "by_type": defaultdict(int, {
                stub.stub_type: sum(1 for s in stubs if s.stub_type == stub.stub_type)
                for stub in stubs
            })
        }
    
    def generate_report(self) -> str:
        analysis = self.analyze()
        stubs = self.find_stubs()
        
        report = f"""# Stub Implementation Analysis Report
Generated: {datetime.now().strftime('%Y-%m-%d %H:%M:%S')}

## Executive Summary
- **Total Stub Implementations**: {analysis['total_stubs']}
- **Critical Issues**: {analysis['by_severity'].get('CRITICAL', 0)}
- **High Priority Issues**: {analysis['by_severity'].get('HIGH', 0)}
- **Medium Priority Issues**: {analysis['by_severity'].get('MEDIUM', 0)}

## Most Problematic Files
"""
        for file_path, count in analysis['problematic_files']:
            report += f"- `{file_path}`: {count} stubs\n"
        
        report += "\n## Critical Functions Requiring Immediate Attention\n"
        
        # Group critical functions by file
        critical_by_file = defaultdict(list)
        for func in analysis['critical_functions']:
            critical_by_file[func.file_path].append(func)
        
        for file_path, funcs in sorted(critical_by_file.items()):
            report += f"\n### {file_path}\n"
            for func in funcs:
                report += f"- **{func.function_name}** (line {func.line_number}): {func.line_content}\n"
        
        report += "\n## Stub Type Distribution\n"
        for stub_type, count in sorted(analysis['by_type'].items(), key=lambda x: x[1], reverse=True):
            description = self.stub_patterns[stub_type]['description']
            report += f"- {description}: {count} instances\n"
        
        report += "\n## Recommended Action Plan\n\n"
        report += self._generate_action_plan(stubs)
        
        return report
    
    def _generate_action_plan(self, stubs: List[StubInstance]) -> str:
        # Identify specific high-priority fixes
        import_export_stubs = [s for s in stubs if 'handlers/imports.go' in s.file_path or 'handlers/exports.go' in s.file_path]
        graphql_stubs = [s for s in stubs if 'schema.resolvers.go' in s.file_path]
        export_gen_stubs = [s for s in stubs if 'export-generator/main.go' in s.file_path]
        
        plan = "### Phase 1: Critical Fixes (1-2 days)\n"
        plan += "1. **Fix Import/Export Handlers**\n"
        if import_export_stubs:
            plan += "   - Implement `getUserImportJobs()` and `getUserExportJobs()` with proper DynamoDB queries\n"
            plan += "   - These functions currently return empty arrays, breaking the entire import/export feature\n"
        
        plan += "\n2. **Fix Export Data Generation**\n"
        if export_gen_stubs:
            plan += "   - Implement all `get*()` functions in export-generator/main.go\n"
            plan += "   - Currently exports generate empty files\n"
        
        plan += "\n### Phase 2: High Priority (3-5 days)\n"
        if graphql_stubs:
            plan += "3. **GraphQL API**\n"
            plan += "   - Replace all panic statements with proper implementations\n"
            plan += "   - Start with query resolvers before mutations\n"
        
        plan += "\n4. **Trends System**\n"
        plan += "   - Implement GSI queries for trends functionality\n"
        
        plan += "\n### Phase 3: Feature Completion (1-2 weeks)\n"
        plan += "5. **Media Processing**\n"
        plan += "   - Complete video and audio processing implementations\n"
        plan += "\n6. **Notifications**\n"
        plan += "   - Wire up the notification system\n"
        plan += "\n7. **Search**\n"
        plan += "   - Implement hashtag search functionality\n"
        
        return plan
    
    def generate_jira_tickets(self) -> List[Dict]:
        """Generate JIRA ticket descriptions for the found issues."""
        analysis = self.analyze()
        stubs = self.find_stubs()
        
        tickets = []
        
        # Group stubs by feature area
        feature_groups = defaultdict(list)
        for stub in stubs:
            if stub.severity in ['CRITICAL', 'HIGH']:
                if 'import' in stub.file_path.lower() or 'export' in stub.file_path.lower():
                    feature_groups['Import/Export'].append(stub)
                elif 'graphql' in stub.file_path.lower() or 'resolver' in stub.file_path.lower():
                    feature_groups['GraphQL'].append(stub)
                elif 'trend' in stub.file_path.lower():
                    feature_groups['Trends'].append(stub)
                elif 'media' in stub.file_path.lower():
                    feature_groups['Media'].append(stub)
                else:
                    feature_groups['Other'].append(stub)
        
        for feature, stubs in feature_groups.items():
            if not stubs:
                continue
                
            # Find unique files and functions
            files = list(set(s.file_path for s in stubs))
            functions = list(set(s.function_name for s in stubs if s.function_name))
            
            ticket = {
                "title": f"Fix stub implementations in {feature} functionality",
                "description": f"""## Overview
The {feature} functionality contains {len(stubs)} stub implementations that need to be properly implemented.

## Affected Files
{chr(10).join(f'- {f}' for f in files[:5])}
{f'... and {len(files) - 5} more files' if len(files) > 5 else ''}

## Affected Functions
{chr(10).join(f'- {f}()' for f in functions[:10])}
{f'... and {len(functions) - 10} more functions' if len(functions) > 10 else ''}

## Impact
- Users cannot use {feature} features properly
- Functions return empty data or panic instead of working correctly

## Acceptance Criteria
- [ ] All identified stub functions are properly implemented
- [ ] Unit tests are added for the implemented functions
- [ ] Integration tests pass
- [ ] No more "For now" or "would normally" comments in the affected code
""",
                "priority": "CRITICAL" if any(s.severity == "CRITICAL" for s in stubs) else "HIGH",
                "labels": ["technical-debt", "stub-implementation", feature.lower()]
            }
            tickets.append(ticket)
        
        return tickets


def main():
    analyzer = StubAnalyzer()
    
    print("Analyzing codebase for stub implementations...")
    report = analyzer.generate_report()
    
    # Save report
    with open("stub_analysis_report.md", "w") as f:
        f.write(report)
    print("\nReport saved to: stub_analysis_report.md")
    
    # Generate JIRA tickets
    tickets = analyzer.generate_jira_tickets()
    with open("jira_tickets.json", "w") as f:
        json.dump(tickets, f, indent=2)
    print(f"Generated {len(tickets)} JIRA ticket templates in: jira_tickets.json")
    
    # Print summary
    analysis = analyzer.analyze()
    print(f"\nSummary:")
    print(f"- Total stubs found: {analysis['total_stubs']}")
    print(f"- Critical issues: {analysis['by_severity'].get('CRITICAL', 0)}")
    print(f"- High priority issues: {analysis['by_severity'].get('HIGH', 0)}")
    print(f"\nTop 3 files with most stubs:")
    for file_path, count in analysis['problematic_files'][:3]:
        print(f"  - {file_path}: {count} stubs")


if __name__ == "__main__":
    main() 