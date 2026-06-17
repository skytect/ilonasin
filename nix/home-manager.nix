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
  tokenFileType = lib.types.nullOr (lib.types.either lib.types.path lib.types.str);
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

    client = {
      host = lib.mkOption {
        type = lib.types.str;
        default = "127.0.0.1";
        description = "Host local clients should use to reach ilonasin.";
      };

      port = lib.mkOption {
        type = lib.types.port;
        default = 11435;
        description = "Port local clients should use to reach ilonasin.";
      };

      baseUrl = lib.mkOption {
        type = lib.types.str;
        default = "http://${cfg.client.host}:${toString cfg.client.port}";
        description = "Base URL for local clients, without an API-version suffix.";
      };

      openAIBaseUrl = lib.mkOption {
        type = lib.types.str;
        default = "${cfg.client.baseUrl}/v1";
        description = "OpenAI-compatible base URL for clients that expect a /v1 endpoint.";
      };

      tokenFile = lib.mkOption {
        type = tokenFileType;
        default = null;
        description = ''
          Optional file containing an ilonasin local client token. The file is
          managed by the caller, for example with sops-nix. Ilonasin itself
          stores only token hashes in SQLite.
        '';
      };

      codex = {
        providerName = lib.mkOption {
          type = lib.types.str;
          default = "ilonasin";
          description = "Codex model provider name for the generated provider config.";
        };

        wireAPI = lib.mkOption {
          type = lib.types.enum [
            "responses"
            "chat"
          ];
          default = "responses";
          description = "Codex wire API for the generated provider config.";
        };

        modelProvider = lib.mkOption {
          type = lib.types.attrs;
          readOnly = true;
          default = {
            name = cfg.client.codex.providerName;
            base_url = cfg.client.openAIBaseUrl;
            wire_api = cfg.client.codex.wireAPI;
          }
          // lib.optionalAttrs (cfg.client.tokenFile != null) {
            auth = {
              command = "${pkgs.coreutils}/bin/cat";
              args = [ (toString cfg.client.tokenFile) ];
            };
          };
          description = ''
            Codex model_providers entry for ilonasin. Set
            services.ilonasin.client.tokenFile to include bearer auth.
          '';
        };
      };
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
