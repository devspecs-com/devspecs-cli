# Create Agent Task

This task guides you through creating a new BMAD agent following the standard template.

## Prerequisites

- Agent template: `.bmad-core/templates/agent-tmpl.md`
- Target directory: `.bmad-core/agents/`

## Steps

### 1. Gather Agent Information

Collect the following information from the user:

- **Agent ID**: Unique identifier (lowercase, hyphens allowed, e.g., `data-analyst`)
- **Agent Name**: Display name (e.g., `Data Analyst`)
- **Agent Title**: Professional title (e.g., `Data Analysis Specialist`)
- **Role Description**: Brief description of the agent's primary role
- **Communication Style**: How the agent communicates (e.g., `analytical, data-driven, clear`)
- **Identity**: Detailed description of who this agent is
- **Focus Areas**: Primary areas of expertise and focus
- **Core Principles**: 3-5 guiding principles for the agent
- **Customization**: Optional specific behaviors or overrides

### 2. Define Agent Capabilities

**IMPORTANT**:

- If your agent will perform any actions → You MUST create corresponding tasks in `.bmad-core/tasks/`
- If your agent will create any documents → You MUST create templates in `.bmad-core/templates/` AND include the `create-doc` task

Determine:

- **Custom Commands**: Agent-specific commands beyond the defaults
- **Required Tasks**: Tasks from `.bmad-core/tasks/` the agent needs
  - For any action the agent performs, a corresponding task file must exist
  - Always include `create-doc` if the agent creates any documents
- **Required Templates**: Templates from `.bmad-core/templates/` the agent uses
  - For any document the agent can create, a template must exist
- **Required Checklists**: Checklists the agent references
- **Required Data**: Data files the agent needs access to
- **Required Utils**: Utility files the agent uses

### 3. Handle Missing Dependencies

**Protocol for Missing Tasks/Templates:**

1. Check if each required task/template exists
2. For any missing items:
   - Create a basic version following the appropriate template
   - Track what was created in a list
3. Continue with agent creation
4. At the end, present a summary of all created items

**Track Created Items:**

```text
Created during agent setup:
- Tasks:
  - [ ] task-name-1.md
  - [ ] task-name-2.md
- Templates:
  - [ ] template-name-1.md
  - [ ] template-name-2.md
```

### 4. Create Agent File

1. Copy the template from `.bmad-core/templates/agent-tmpl.md`
2. Replace all placeholders with gathered information:

   - `[AGENT_ID]` → agent id
   - `[AGENT_NAME]` → agent name
   - `[AGENT_TITLE]` → agent title
   - `[AGENT_ROLE_DESCRIPTION]` → role description
   - `[COMMUNICATION_STYLE]` → communication style
   - `[AGENT_IDENTITY_DESCRIPTION]` → identity description
   - `[PRIMARY_FOCUS_AREAS]` → focus areas
   - `[PRINCIPLE_X]` → core principles
   - `[OPTIONAL_CUSTOMIZATION]` → customization (or remove if none)
   - `[DEFAULT_MODE_DESCRIPTION]` → description of default chat mode
   - `[STARTUP_INSTRUCTIONS]` → what the agent should do on activation
   - Add custom commands, tasks, templates, etc.

3. Save as `.bmad-core/agents/[agent-id].md`

### 4. Validate Agent

Ensure:

- All placeholders are replaced
- Dependencies (tasks, templates, etc.) actually exist
- Commands are properly formatted
- YAML structure is valid

### 5. Build and Test

1. Run `npm run build:agents` to include in builds
2. Test agent activation and commands
3. Verify all dependencies load correctly

### 6. Final Summary

Present to the user:

```text
✅ Agent Created: [agent-name]
   Location: .bmad-core/agents/[agent-id].md

📝 Dependencies Created:
   Tasks:
   - ✅ task-1.md - [brief description]
   - ✅ task-2.md - [brief description]

   Templates:
   - ✅ template-1.md - [brief description]
   - ✅ template-2.md - [brief description]

⚠️  Next Steps:
   1. Review and customize the created tasks/templates
   2. Run npm run build:agents
   3. Test the agent thoroughly
```

## Template Reference

The agent template structure:

- **activation-instructions**: How the AI should interpret the file
- **agent**: Basic agent metadata
- **persona**: Character and behavior definition
- **startup**: Initial actions on activation
- **commands**: Available commands (always include defaults)
- **dependencies**: Required resources organized by type

## Example Usage

<!-- stripped fenced code block: yaml -->

## Creating Missing Dependencies

When a required task or template doesn't exist:

1. **For Missing Tasks**: Create using `.bmad-core/templates/task-template.md`

   - Name it descriptively (e.g., `analyze-metrics.md`)
   - Define clear steps for the action
   - Include any required inputs/outputs

2. **For Missing Templates**: Create a basic structure

   - Name it descriptively (e.g., `metrics-report-template.md`)
   - Include placeholders for expected content
   - Add sections relevant to the document type

3. **Always Track**: Keep a list of everything created to report at the end

## Important Reminders

### Tasks and Templates Requirement

- **Every agent action needs a task**: If an agent can "analyze data", there must be an `analyze-data.md` task
- **Every document type needs a template**: If an agent can create reports, there must be a `report-template.md`
- **Document creation requires**: Both the template AND the `create-doc` task in dependencies

### Example Dependencies

<!-- stripped fenced code block: yaml -->

## Notes

- Keep agent definitions focused and specific
- Ensure dependencies are minimal and necessary
- Test thoroughly before distribution
- Follow existing agent patterns for consistency
- Remember: No task = agent can't do it, No template = agent can't create it
