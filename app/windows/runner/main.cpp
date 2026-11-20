#include <flutter/dart_project.h>
#include <flutter/flutter_view_controller.h>
#include <flutter_windows.h>
#include <windows.h>

#include "flutter_window.h"
#include "utils.h"

int APIENTRY wWinMain(_In_ HINSTANCE instance, _In_opt_ HINSTANCE prev,
                      _In_ wchar_t *command_line, _In_ int show_command) {
  HANDLE single = ::CreateMutexW(nullptr, FALSE, L"Local\\app.spofy.window");
  if (single != nullptr && ::GetLastError() == ERROR_ALREADY_EXISTS) {
    HWND shown = ::FindWindowW(L"SPOFY_WINDOW", nullptr);
    if (shown != nullptr) {
      ::ShowWindow(shown, ::IsIconic(shown) ? SW_RESTORE : SW_SHOW);
      ::SetForegroundWindow(shown);
    }
    return EXIT_SUCCESS;
  }

  if (!::AttachConsole(ATTACH_PARENT_PROCESS) && ::IsDebuggerPresent()) {
    CreateAndAttachConsole();
  }

  ::CoInitializeEx(nullptr, COINIT_APARTMENTTHREADED);

  flutter::DartProject project(L"data");

  std::vector<std::string> command_line_arguments =
      GetCommandLineArguments();

  project.set_dart_entrypoint_arguments(std::move(command_line_arguments));

  FlutterWindow window(project);
  RECT area{0, 0, 1280, 720};
  ::SystemParametersInfoW(SPI_GETWORKAREA, 0, &area, 0);
  double scale = FlutterDesktopGetDpiForMonitor(::MonitorFromPoint({0, 0}, MONITOR_DEFAULTTOPRIMARY)) / 96.0;
  int wide = static_cast<int>((area.right - area.left) / scale);
  int high = static_cast<int>((area.bottom - area.top) / scale);
  int tall = high - 80 > 800 ? 800 : (high - 80 < 600 ? 600 : high - 80);
  int left = static_cast<int>(area.left / scale) + (wide - 420) / 2;
  int top = static_cast<int>(area.top / scale) + (high - tall) / 2;
  Win32Window::Point origin(left > 0 ? left : 0, top > 0 ? top : 0);
  Win32Window::Size size(420, tall);
  if (!window.Create(L"Spofy", origin, size)) {
    return EXIT_FAILURE;
  }
  window.SetQuitOnClose(true);

  ::MSG msg;
  while (::GetMessage(&msg, nullptr, 0, 0)) {
    ::TranslateMessage(&msg);
    ::DispatchMessage(&msg);
  }

  ::CoUninitialize();
  return EXIT_SUCCESS;
}
