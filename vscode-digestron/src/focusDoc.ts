import * as vscode from "vscode";

export const FOCUS_PACK_URI = vscode.Uri.parse("digestron://focuspack/last");

export class FocusDocProvider implements vscode.TextDocumentContentProvider {
  private _onDidChange = new vscode.EventEmitter<vscode.Uri>();
  readonly onDidChange = this._onDidChange.event;

  private content = "";

  setContent(text: string) {
    this.content = text || "";
    this._onDidChange.fire(FOCUS_PACK_URI);
  }

  provideTextDocumentContent(_uri: vscode.Uri): string {
    return this.content || "";
  }
}
