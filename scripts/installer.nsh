; Included by the MyGo (NSIS) installer: windows.installerInclude in mygo.config.ts.
; customUnInstall runs in the uninstaller before the install folder is removed.
!macro customUnInstall
  ; Settings (theme, last speed mode) live in %APPDATA%\Rokit; keep them unless the user says otherwise.
  IfSilent keep
  MessageBox MB_YESNO|MB_ICONQUESTION|MB_DEFBUTTON2 "Hapus juga pengaturan Rokit di $APPDATA\Rokit?$\r$\n(Pilih Tidak untuk menyimpannya.)" IDNO keep
  RMDir /r "$APPDATA\Rokit"
  keep:
!macroend
