!macro customUnInstall
  ; Updates and silent uninstall always preserve data. Only an interactive,
  ; explicit choice deletes this application's own fixed user-data directory.
  ${IfNot} ${isUpdated}
    ${IfNot} ${Silent}
      MessageBox MB_YESNO|MB_ICONQUESTION|MB_DEFBUTTON1 "保留 AI Hub 的本地消耗记录、设置和登录信息吗？$\r$\n$\r$\n选择 是：重新安装后继续使用（推荐）。$\r$\n选择 否：永久删除此用户的 AI Hub 本地数据。" IDYES aihub_keep_data
      SetShellVarContext current
      RMDir /r "$APPDATA\aihub-desktop"
      aihub_keep_data:
    ${EndIf}
  ${EndIf}
!macroend
