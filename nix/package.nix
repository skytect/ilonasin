{
  lib,
  buildGoModule,
  pkg-config,
  sqlite,
  commit ? "",
  commitSubject ? "",
}:

let
  src = lib.cleanSource ../.;
  version = "0.1.0";
in
buildGoModule {
  pname = "ilonasin";
  inherit version;

  inherit src;

  vendorHash = "sha256-gBP25CkwLRKRuZQB/2E/5JwE29GBrmJe8pe49hXBVpw=";

  subPackages = [ "cmd/ilonasin" ];

  nativeBuildInputs = [ pkg-config ];
  buildInputs = [ sqlite ];

  env.CGO_ENABLED = 1;

  ldflags = [
    "-s"
    "-w"
    "-X ilonasin/internal/cli.Version=${version}"
    "-X ilonasin/internal/cli.Commit=${commit}"
    "-X ilonasin/internal/cli.CommitSubject=${commitSubject}"
  ];

  meta = {
    description = "Local OpenAI-compatible LLM router";
    homepage = "https://github.com/skytect/ilonasin";
    license = lib.licenses.mit;
    mainProgram = "ilonasin";
  };
}
