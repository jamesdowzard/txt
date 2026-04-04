import SwiftUI

@main
struct OpenMessageApp: App {
    @NSApplicationDelegateAdaptor(AppDelegate.self) var appDelegate
    @StateObject private var backend: BackendManager
    @StateObject private var notifications: NotificationManager
    @StateObject private var contacts: ContactsManager

    init() {
        let backend = BackendManager()
        self._backend = StateObject(wrappedValue: backend)
        self._notifications = StateObject(wrappedValue: NotificationManager(baseURL: backend.baseURL))
        self._contacts = StateObject(wrappedValue: ContactsManager())
    }

    var body: some Scene {
        Window("OpenMessage", id: "main") {
            ContentView(backend: backend, notifications: notifications, contacts: contacts)
                .frame(minWidth: 800, minHeight: 500)
        }
        .defaultSize(width: 1100, height: 700)

        MenuBarExtra("OpenMessage", systemImage: "message.fill") {
            MenuBarView(backend: backend)
        }
    }
}

final class AppDelegate: NSObject, NSApplicationDelegate, @unchecked Sendable {
    func applicationShouldTerminateAfterLastWindowClosed(_ sender: NSApplication) -> Bool {
        false // Keep running in menu bar when window closed
    }
}
