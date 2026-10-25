class AppConfig {
  static const defaultServerUrl = String.fromEnvironment('AIHUB_DEFAULT_SERVER',
      defaultValue: 'http://127.0.0.1:8080');
  static const defaultProfileId = 'public-cloud';
  static const defaultProfileName = String.fromEnvironment(
      'AIHUB_DEFAULT_PROFILE_NAME',
      defaultValue: '本机开发');
}
