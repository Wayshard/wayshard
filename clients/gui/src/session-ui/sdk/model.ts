// Wayshard client view-model used by the adapted session presentation layer.
// The imported OpenCode session components render a message/part stream;
// Wayshard owns the domain, so these view-model shapes are populated from
// @wayshard/sdk data. No OpenCode runtime dependency.
export interface FileDiffInfo { file: string; path?: string; additions: number; deletions: number; [key: string]: any }
export interface SnapshotFileDiff { file: string; path?: string; additions: number; deletions: number; [key: string]: any }
export interface VcsFileDiff { file: string; path?: string; additions: number; deletions: number; [key: string]: any }
export interface FileContent { path: string; content: string; hash?: string; binary?: boolean; [key: string]: any }
export interface FilePartSource { [key: string]: any }
export interface FilePart { id: string; type: "file"; [key: string]: any }
export interface ToolPart { id: string; type: "tool"; [key: string]: any }
export interface AgentPart { id: string; type: "agent"; [key: string]: any }
export interface ReasoningPart { id: string; type: "reasoning"; [key: string]: any }
export interface TextPart { id: string; type: "text"; [key: string]: any }
export interface Todo { [key: string]: any }
export interface QuestionAnswer { [key: string]: any }
export interface QuestionInfo { [key: string]: any }
export interface Provider { id: string; [key: string]: any }
export interface Session { id: string; title: string; [key: string]: any }
export interface SessionStatus { type?: string; [key: string]: any }
export interface UserMessage { id: string; role: "user"; [key: string]: any }
export interface AssistantMessage { id: string; role: "assistant"; parentID?: string; time: { completed?: number; [key: string]: any }; [key: string]: any }
export type Message = UserMessage | AssistantMessage
export interface GenericPart { id: string; type: string; [key: string]: any }
export type Part = ToolPart | FilePart | TextPart | ReasoningPart | AgentPart | GenericPart
