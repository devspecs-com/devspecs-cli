Okay, here's a Product Requirements Document (PRD) for rebuilding the "Task Analyser" as a frontend-only Angular application.

## Product Requirements Document: Angular Task Analyser

**1. Introduction**

The Angular Task Analyser is a client-side web application designed to help users break down complex tasks into a hierarchical, manageable structure. It leverages Large Language Models (LLMs) for automated sub-task generation and provides tools for manual editing, organization, and visualization of the task tree. This project aims to modernize the existing jQuery-based Task Analyser by rebuilding it with Angular, focusing on a component-based architecture, improved state management, and a more maintainable codebase, while retaining all core functionalities and the visual design of task nodes.

**2. Goals**

*   Develop a robust, maintainable, and scalable frontend-only Angular application.
*   Replicate all core features of the existing Task Analyser.
*   Improve user experience through a modern Angular architecture.
*   Ensure data persistence via JSON file import/export and potentially browser local storage.
*   Maintain the existing dark theme and node styling for visual consistency.
*   Provide a clear path for users to configure LLM providers and initiate task analysis.

**3. Target Audience**

*   Project Managers
*   Software Developers
*   Students
*   Content Creators
*   Anyone needing to break down complex tasks into smaller, actionable steps.

**4. User Stories**

*   **US01 (LLM Config):** As a user, I want to configure my preferred LLM provider, model name, API key, and API endpoint (if applicable) so that the application can use my chosen LLM for task breakdown.
*   **US02 (LLM Config Save):** As a user, I want my LLM configuration (excluding API key) to be saved (e.g., in local storage) so I don't have to re-enter it every time.
*   **US03 (New Task):** As a user, I want to create a new task analysis project by providing a project name, project description, an overall task summary, and optionally, the target technology stack, so the LLM can generate relevant sub-tasks.
*   **US04 (Load Existing):** As a user, I want to load an existing task breakdown from a `task_tree.json` file so I can continue working on a previous project.
*   **US05 (View Hierarchy):** As a user, I want to see the task hierarchy displayed as a nested, flowchart-like structure so I can easily understand the relationships between tasks.
*   **US06 (Node Details):** As a user, I want to see the text and properties of each task node clearly displayed.
*   **US07 (Edit Node):** As a user, I want to edit the text and properties (JSON format) of any task node so I can correct or update information.
*   **US08 (Delete Node):** As a user, I want to delete any task node so I can remove irrelevant or incorrect tasks.
*   **US09 (Subdivide Node - LLM):** As a user, I want to further subdivide an existing task node using the configured LLM so I can get more granular sub-tasks.
*   **US10 (Add Child Node):** As a user, I want to manually add a new child task node to an existing node so I can expand the task breakdown.
*   **US11 (Add Sibling Node):** As a user, I want to manually add a new sibling task node above or below an existing node.
*   **US12 (Move Node):** As a user, I want to move a task node up or down within its current parent, or move it to its parent's level (outdent), so I can re-organize the task structure.
*   **US13 (Add Property):** As a user, I want to add a custom key-value property to a task node so I can store additional metadata.
*   **US14 (Export Project):** As a user, I want to export the current task tree and LLM configuration (excluding API key) to a `task_tree.json` file so I can save my work or share it.
*   **US15 (Visual Consistency):** As a user, I want the task nodes (icons, actions, layout) to look and feel the same as in the original application for familiarity.
*   **US16 (Navigation):** As a user, I want a clear home page with navigation to LLM settings, creating a new task, or loading an existing task.

**5. Proposed Features (Angular Component-Based)**

**5.1. Core/Shared**
    *   **Angular Routing:** To navigate between Home, LLM Settings, and Task Analyser views.
    *   **State Management Service (`TaskTreeService`):**
        *   Holds the main `taskTree` object (root node and its children).
        *   Methods for all node manipulations (add, edit, delete, move, add property, find node, find parent).
        *   Manages `prompts_and_responses` for each node.
        *   Handles JSON import/export logic.
        *   Optionally, handles saving/loading the `taskTree` to/from local storage.
    *   **LLM Service (`LlmService`):**
        *   Manages LLM configurations (provider, model, key, endpoint) - potentially interacts with a `ConfigService` or local storage.
        *   Contains `createPrompt` logic (migrated from `llmService.js` and `prompts.js`).
        *   Handles `subDivideTaskLLM` API calls to various LLM providers.
    *   **Configuration Service (`ConfigService`):**
        *   Manages storage and retrieval of LLM settings (provider, model, endpoint - API key handled ephemerally or with user consent for local storage).
    *   **Notification Service:** For displaying success/error messages (e.g., using toasts).
    *   **Global Styles:** Dark theme, common utility classes. Bootstrap can be used for base styling and layout.

**5.2. Components**

    *   **`AppComponent` (Shell):**
        *   Root component.
        *   Contains the main router outlet.
        *   May include a simple header/navbar if needed for global navigation beyond the home page.

    *   **`HomeComponent`:**
        *   Landing page of the application.
        *   Displays:
            *   Link/Button to navigate to "LLM Provider Settings".
            *   Link/Button to "Create a New Task Analysis".
            *   Button/Input to "Load Existing Task Tree (`task_tree.json`)".

    *   **`LlmSettingsComponent`:**
        *   Accessible from `HomeComponent`.
        *   Form fields:
            *   LLM Provider (Dropdown: OpenAI, Groq, Ollama, Other).
            *   Model Name (Text input).
            *   API Key (Password input with show/hide toggle).
            *   API Endpoint (Text input, shown only if "Other" provider is selected).
        *   Save button: Persists settings (excluding API key ideally, or with explicit user consent if API key is stored in local storage) using `ConfigService`.
        *   Loads existing settings on initialization.

    *   **`TaskAnalyserComponent` (Main View):**
        *   Loaded when a new task is created or an existing one is loaded.
        *   Displays the task tree using a recursive `TaskNodeComponent`.
        *   Contains global action buttons: "Export JSON".
        *   If a new task is being created, it will initially host/embed the `NewTaskFormComponent`.

    *   **`NewTaskFormComponent`:**
        *   Form to collect initial task information:
            *   Project Name (Text input).
            *   Project Description (Textarea).
            *   Overall Task Summary (Textarea - this will be the root node's text).
            *   Target Technology Stack (e.g., comma-separated tags, multi-select dropdown - for refining LLM prompts).
        *   "Start Dissembling" button:
            *   Collects form data.
            *   Saves project info to `TaskTreeService` (or passes it up).
            *   Creates the root node in the `taskTree`.
            *   Triggers the initial LLM call via `LlmService` to subdivide the root task.
            *   Navigates/switches view to show the `TaskAnalyserComponent` with the generated tree.

    *   **`TaskNodeComponent` (Recursive):**
        *   Inputs: `nodeData` (the task node object), `parentNodeData` (optional, for context).
        *   Displays:
            *   Node text.
            *   Node properties (conditionally, or on expand).
            *   Node actions (icons as per existing design):
                *   Edit (`<i class="fas fa-edit">`): Opens `EditNodeModalComponent`.
                *   Delete (`<i class="fas fa-trash">`): Confirmation then calls `TaskTreeService`.
                *   Subdivide with options (`<i class="fas fa-code-branch">`): Triggers LLM call via `LlmService` (potentially with a small modal for custom prompt additions).
                *   Subdivide with default (`<i class="fas fa-sitemap">`): Triggers LLM call.
                *   Add Property (`<i class="fas fa-plus-circle">`): Opens `AddPropertyModalComponent`.
                *   Add Child (`<i class="fas fa-plus">`): Opens `AddNodeModalComponent` (pre-filled for child).
                *   Move Up (`<i class="fas fa-arrow-up">`): Calls `TaskTreeService`.
                *   Move Down (`<i class="fas fa-arrow-down">`): Calls `TaskTreeService`.
                *   Move to Parent Level (`<i class="fas fa-level-up-alt">`): Calls `TaskTreeService`.
                *   Add Task Above (`<i class="fas fa-arrow-circle-up">`): Opens `AddNodeModalComponent` (pre-filled for sibling above).
                *   Add Task Below (`<i class="fas fa-arrow-circle-down">`): Opens `AddNodeModalComponent` (pre-filled for sibling below).
        *   Styling for node structure and connecting lines (CSS, similar to `styles.css`).
        *   Recursively renders `<app-task-node>` for each child in `nodeData.children`.

    *   **Modal Components (using a library like `ng-bootstrap` or Angular Material Dialogs):**
        *   **`EditNodeModalComponent`:**
            *   Input: `nodeData`.
            *   Form to edit node text and properties (textarea for JSON).
            *   Save/Cancel buttons. Calls `TaskTreeService` on save.
        *   **`AddNodeModalComponent`:**
            *   Input: `parentNodeId`, `siblingNodeId` (optional), `position` ('child', 'sibling-above', 'sibling-below').
            *   Form for new node text and initial properties (JSON).
            *   Save/Cancel buttons. Calls `TaskTreeService` on save.
        *   **`AddPropertyModalComponent` (Optional, could be part of EditNodeModal):**
            *   Input: `nodeId`.
            *   Form for property key and value.
            *   Save/Cancel. Calls `TaskTreeService`.

**5.3. Data Structures**
    *   **Task Node Object:** `id`, `text`, `description`, `children: TaskNode[]`, `properties: { [key: string]: any }`, `prompts_and_responses: {prompt: any, response: string}[]`. (Other properties from the original `ecommerce-ui-plan.json` can be included if deemed useful by the LLM or user).
    *   **LLM Configuration Object:** `provider`, `modelName`, `apiKey` (handled carefully), `apiEndpoint`.

**6. Non-Functional Requirements**

*   **Performance:** The application should remain responsive, especially when rendering and manipulating large task trees.
*   **Usability:** Intuitive navigation and clear visual feedback for user actions.
*   **Maintainability:** Well-structured, commented Angular code.
*   **Browser Compatibility:** Support for latest versions of major browsers (Chrome, Firefox, Edge, Safari).
*   **Accessibility:** Basic accessibility considerations (e.g., keyboard navigation for modals, sufficient color contrast).
*   **Client-Side Only:** No server-side backend dependencies for core functionality.

**7. Out of Scope (for initial version)**

*   User authentication and accounts.
*   Server-side data persistence.
*   Real-time collaboration features.
*   Advanced analytics or reporting.
*   Automated testing (though highly recommended for subsequent iterations).

**8. Success Metrics**

*   Successful migration of all core features from the jQuery version.
*   Positive user feedback on usability and performance.
*   Ability for users to successfully create, manage, import, and export task trees.
*   Low error rates during LLM interactions and node manipulations.

**9. Future Considerations**

*   Integration with version control systems (e.g., Git for task history).
*   More sophisticated prompt templating and management within the UI.
*   Visual diffing for changes in task trees.
*   Export to other formats (e.g., Markdown, PDF).
*   Drag-and-drop node reordering.
*   Enhanced local storage options with multiple project slots.

**10. Migration Notes from Existing Codebase**

*   **`globalData.js`:** State will be managed by Angular services.
*   **`prompts.js`:** Logic/data will be incorporated into the `LlmService`.
*   **`llmService.js`:** Functionality will be part of the Angular `LlmService`.
*   **`treeDataManipulation.js`:** Logic will reside in the `TaskTreeService`.
*   **`flowchartRenderer.js`:** Rendering will be handled by the recursive `TaskNodeComponent` template and its CSS.
*   **`rootFunctions.js`:** Initialization and global actions will be distributed among relevant components and services.
*   **`popupHandlerNodeAddEdit.js` & `popupHandler.js`:** Modal logic will be handled by Angular modal components.
*   **`serverCalls.js`:** Largely irrelevant as this is frontend-only. Import/export logic moves to `TaskTreeService`.
*   **CSS:** `styles.css` and `sidebar.css` (if sidebars are desired for navigation) can be adapted. Node-specific styles will be part of `TaskNodeComponent`'s CSS.
*   **`index.html`:** The structure will be entirely new, based on Angular components.
*   **`extra/` files:** JSON examples are useful for testing import/export. HTML prototypes for sidebars might inform navigation design if used.

This PRD provides a comprehensive plan for developing the Angular Task Analyser. The component-based approach should lead to a more organized and maintainable application.
