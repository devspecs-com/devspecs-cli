# Request for Comments (RFC)

## 1. Overview

This RFC outlines the technical architecture and implementation details for the Chrome Extension that alerts users when visiting boycotted websites. Feedback is requested on the proposed design, potential edge cases, and any improvements that can be made.

---

## 2. Technical Design

### 2.1 Architecture

The extension is divided into the following components:

- **Background Service Worker:**

  - Monitors all navigation events using Chrome APIs.
  - Checks each navigated URL against the stored list of boycotted sites.
  - Initiates alert notifications if a match is found.

- **Content Script:**

  - Injected into web pages when a boycotted site is detected.
  - Responsible for displaying the alert modal with clear information and options (proceed/go back).

- **Popup UI:**

  - Provides a user interface accessible from the extension icon.
  - Displays the current list of boycotted sites.
  - Allows quick access to the options/settings page.

- **Options Page:**

  - Enables users to view and modify the list of boycotted sites.
  - Allows configuration of alert behaviors (e.g., toggling alerts, customizing messages).

- **Data Persistence:**
  - Uses `chrome.storage.sync` (or `chrome.storage.local`) to store the boycotted sites list and user settings.
  - Potential future integration with an external API for real-time updates.

### 2.2 Workflow

1. **Navigation Detection:**
   - The background service worker listens for navigation events (using `chrome.webNavigation` or `chrome.tabs.onUpdated`).
2. **URL Matching:**
   - When a new URL is detected, it is compared against the locally stored list of boycotted sites.
3. **Alert Initiation:**
   - If the URL matches a boycotted site, a message is sent to the content script to display an alert.
4. **User Interaction:**
   - The alert modal provides options for the user to either continue or return to safety.
5. **Logging & Metrics:**
   - Optional logging can track how often alerts are triggered and user choices for future improvements.

---

## 3. Design Considerations

- **Manifest V3 Compliance:**
  - Ensuring all components (background, content, popup) work seamlessly with Manifest V3.
- **Minimal Permissions:**

  - Request only the necessary permissions (e.g., `tabs`, `webNavigation`, `storage`) to reduce security risks.

- **Performance Optimization:**

  - The extension should not noticeably affect the browsing experience.
  - Use efficient matching algorithms to check URLs against the list.

- **User Customization:**

  - Allow users to update the list manually.
  - Consider an automated update mechanism via an external API in future versions.

- **Security & Privacy:**
  - Follow Chrome’s security guidelines.
  - Ensure that any data stored is secure and that user privacy is maintained.

---

## 4. Potential Challenges and Mitigations

- **Edge Cases in URL Matching:**

  - **Challenge:** Variations in URL formatting (e.g., `www.example.com` vs. `example.com`).
  - **Mitigation:** Normalize URLs before comparison and use pattern matching.

- **Alert Fatigue:**

  - **Challenge:** Users might become desensitized if alerts are too frequent.
  - **Mitigation:** Provide options to disable alerts or set thresholds.

- **Browser Compatibility:**
  - **Challenge:** Different versions of Chrome or Chromium-based browsers.
  - **Mitigation:** Test across multiple environments and adhere to Chrome extension best practices.

---

## 5. Testing Strategy

- **Unit Tests:**
  - Test individual components (e.g., URL matching function, storage access).
- **Integration Tests:**
  - Simulate navigation events to ensure the background and content scripts communicate correctly.
- **User Acceptance Testing:**
  - Beta testing with a small group of users to gather feedback on the alert system and overall UX.
- **Security Audits:**
  - Verify that the extension adheres to security best practices and does not expose any vulnerabilities.

---

## 6. Request for Feedback

We welcome comments on:

- The proposed architecture and division of components.
- The URL normalization and matching logic.
- Potential user experience improvements, particularly in managing alert frequency.
- Ideas for future enhancements, such as real-time updates or additional customization options.

---

## 7. Timeline and Next Steps

- **Prototype Development:** 4 weeks
  - Develop the core functionalities (background detection, content script alert, popup UI).
- **Beta Testing:** 2 weeks
  - Release a beta version for initial user feedback.
- **Iteration & Finalization:** 2 weeks
  - Incorporate feedback and finalize the extension.
- **Public Release:** Following successful testing and review.

---
