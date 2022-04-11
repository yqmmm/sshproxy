{
  description = "SSH Proxy";

  outputs = { self, nixpkgs }:
    let

      # to work with older version of flakes
      lastModifiedDate = self.lastModifiedDate or self.lastModified or "19700101";

      # Generate a user-friendly version number.
      version = builtins.substring 0 8 lastModifiedDate;

      # System types to support.
      supportedSystems = [ "x86_64-linux" "x86_64-darwin" "aarch64-linux" "aarch64-darwin" ];

      # Helper function to generate an attrset '{ x86_64-linux = f "x86_64-linux"; ... }'.
      forAllSystems = nixpkgs.lib.genAttrs supportedSystems;

      # Nixpkgs instantiated for supported system types.
      nixpkgsFor = forAllSystems (system: import nixpkgs { inherit system; });

    in
    {

      # Provide some binary packages for selected system types.
      packages = forAllSystems (system:
        let
          pkgs = nixpkgsFor.${system};
        in
        {
          sshproxy = pkgs.buildGoModule {
            pname = "sshproxy";
            inherit version;
            src = ./.;

            vendorSha256 = "sha256-TqWYKFFHWYr+fV6s5VztbqnTXm4TTXFabkzwGQ3sfMU=";
          };
        });

      defaultPackage = forAllSystems (system: self.packages.${system}.sshproxy);
    };
}
