Product Requirements Document: Simple AI Chatbot

1. Project Overview

A minimalist, high-performance AI Chatbot platform allowing users to interact with free LLMs via OpenRouter, with persistent chat history stored in Supabase.

2. Technical Stack

Frontend: React (Next.js), Tailwind CSS, Lucide Icons.

Backend: Python (FastAPI), Uvicorn.

AI Gateway: OpenRouter API (using :free models).

Database/Auth: Supabase (PostgreSQL + Supabase Auth).

3. UI/UX Requirements

Simplicity: High-contrast, clean layout. No glassmorphism. Standard sidebar and main chat area.

Responsive: Mobile-first design with a collapsible sidebar.

Readability: Clean typography (Inter/System fonts) and distinct message bubbles.

Feedback: Clear loading states and streaming text.

4. Functional Requirements

4.1 Authentication

Users must sign in via Supabase Auth (Magic Link or Email).

JWT tokens passed to Python backend for session verification.

4.2 Chat Logic

Streaming: Real-time token streaming from OpenRouter to the UI.

Persistence: All messages saved to Supabase messages table.

Thread Management: Create, delete, and switch between multiple chat threads.

Model Selection: Dropdown to choose between google/gemini-2.0-flash-exp:free and meta-llama/llama-3-8b-instruct:free.

4.3 Database Schema

conversations: [id, user_id, title, model_id, created_at]

messages: [id, conversation_id, role, content, created_at]

5. API Endpoints (FastAPI)

POST /api/chat: Handles streaming completions and saving to DB.

GET /api/conversations: Fetches user's chat history.

DELETE /api/conversations/{id}: Deletes a thread.
