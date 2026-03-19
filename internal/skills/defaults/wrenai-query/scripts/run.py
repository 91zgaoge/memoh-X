#!/usr/bin/env python3
"""
WrenAI Query - Simple wrapper for Claude Code
Usage: python3 "$SKILLS_ROOT/wrenai-query/scripts/run.py" "your query" [instance]
Example: python3 "$SKILLS_ROOT/wrenai-query/scripts/run.py" "查询上个月的KPI指标" pg
"""

import sys
import os

# Get the directory of this script
script_dir = os.path.dirname(os.path.abspath(__file__))

# Import and run the main query module
sys.path.insert(0, script_dir)

# Execute query.py with the same arguments
os.execvp(sys.executable, [sys.executable, os.path.join(script_dir, "query.py")] + sys.argv[1:])
