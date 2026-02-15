# Invalid Skill Without Front Matter

## When to Use
This skill is invalid because it lacks YAML front matter.

## Solution
This skill will fail parsing because it doesn't have the required
front matter section with metadata.

The parser expects the file to start with `---` followed by YAML
metadata, but this file jumps straight into Markdown content.