{ self }:
{
  config,
  lib,
  pkgs,
  ...
}:
let
  cfg = config.services.ilonasin;
  toml = pkgs.formats.toml { };
  package =
    self.packages.${pkgs.stdenv.hostPlatform.system}.default
      or (throw "ilonasin is not packaged for ${pkgs.stdenv.hostPlatform.system}");
  configPath = "${cfg.home}/config.toml";
  configFile = toml.generate "ilonasin-config.toml" cfg.settings;
  escapedHome = lib.escapeShellArg cfg.home;
  escapedConfigPath = lib.escapeShellArg configPath;
in
{
  options.services.ilonasin = {
    enable = lib.mkEnableOption "Ilonasin local OpenAI-compatible LLM router";

    package = lib.mkOption {
      type = lib.types.package;
      default = package;
      defaultText = lib.literalExpression "inputs.ilonasin.packages.${pkgs.stdenv.hostPlatform.system}.default";
      description = "Ilonasin package to install and run.";
    };

    home = lib.mkOption {
      type = lib.types.str;
      default = "${config.home.homeDirectory}/.ilonasin";
      description = "Directory used for ilonasin config, SQLite state, logs, and cache.";
    };

    settings = lib.mkOption {
      type = toml.type;
      default = {
        server.bind = "127.0.0.1:11435";
        paths = {
          data_dir = cfg.home;
          database = "${cfg.home}/ilonasin.sqlite";
          log_dir = "${cfg.home}/logs";
          cache_dir = "${cfg.home}/cache";
        };
        providers = { };
      };
      description = "Ilonasin config.toml contents.";
    };

    service.enable = lib.mkOption {
      type = lib.types.bool;
      default = true;
      description = "Whether to run ilonasin serve as a user systemd service.";
    };
  };

  config = lib.mkIf cfg.enable {
    home.packages = [ cfg.package ];

    home.activation.ilonasin-config = lib.hm.dag.entryAfter [ "writeBoundary" ] ''
      run mkdir -p ${escapedHome}
      run install -m 0600 ${configFile} ${escapedConfigPath}
    '';

    systemd.user.services.ilonasin = lib.mkIf cfg.service.enable {
      Unit = {
        Description = "Ilonasin local OpenAI-compatible LLM router";
        After = [ "network-online.target" ];
        Wants = [ "network-online.target" ];
      };

      Service = {
        Environment = "ILONASIN_HOME=${cfg.home}";
        ExecStart = "${lib.getExe cfg.package} serve --config ${escapedConfigPath}";
        Restart = "on-failure";
        RestartSec = "5s";
      };

      Install.WantedBy = [ "default.target" ];
    };
  };
}
