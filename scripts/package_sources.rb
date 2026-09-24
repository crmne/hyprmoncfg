#!/usr/bin/env ruby
# frozen_string_literal: true

# Hyprmoncfg's distribution source recipes use the shared release helpers.
# AUR recipes are native-packages templates; see native-packages.yaml.
require "native_packages"
require "fileutils"
require "json"
require "open3"
require "openssl"
require "optparse"
require "pathname"
require "time"
require "tmpdir"

module Packages
  extend NativePackages::Support
  extend self

  ROOT = Pathname.new(__dir__).parent.realpath
  RECIPES = ROOT / "packaging"
  UPSTREAM = NativePackages::Configuration.new(ROOT / "native-packages.yaml").project.upstream
  TOKEN = /@([A-Z][A-Z0-9_]*)@/
  HELP = <<~TEXT
    Usage: ruby scripts/package_sources.rb COMMAND

      prepare VERSION [--output DIRECTORY]  Verify assets and generate distribution source recipes
      check DIRECTORY                      Validate a prepared recipe tree
      artifacts DIRECTORY [--output DIR]   Bundle prepared recipes as release assets

    Binary packages, AUR recipes and downstream updates use native-packages; see PACKAGING.md.
  TEXT

  Error = NativePackages::Error

  def root = ROOT
  def package_name = 'hyprmoncfg'
  def upstream = UPSTREAM

  def digests(path)
    hashes = { "sha256" => "SHA256", "sha512" => "SHA512", "md5" => "MD5", "blake2b" => "BLAKE2b512" }
      .transform_values { |name| OpenSSL::Digest.new(name) }
    File.open(path, "rb") do |stream|
      while (chunk = stream.read(1024 * 1024))
        hashes.each_value { |digest| digest.update(chunk) }
      end
    end
    hashes.transform_values(&:hexdigest)
  end

  def verify_assets(checksums, assets)
    expected = {}
    checksums.each_line do |line|
      digest, name = line.strip.split(/\s+/, 2)
      name = name&.delete_prefix("*")
      unless digest && /\A[0-9a-f]{64}\z/.match?(digest) && name && !expected.key?(name)
        raise Error, "invalid or duplicate checksum: #{line.strip}"
      end
      expected[name] = digest
    end
    assets.each do |path|
      unless expected[path.basename.to_s] == digests(path).fetch("sha256")
        raise Error, "checksum mismatch or missing entry for #{path}"
      end
    end
  end

  def extract_archive(archive, directory)
    # Keep bsdtar's default protections against parent paths and symlink traversal.
    run "bsdtar", "-xf", archive, "-C", directory, "--no-same-owner", "--no-same-permissions"
  end

  def nix_vendor_hash(source, deps, version)
    Dir.mktmpdir("hyprmoncfg-vendor-") do |temporary|
      work = Pathname.new(temporary)
      extract_archive(source, work)
      extract_archive(deps, work)
      tree = work / "hyprmoncfg-#{version}"
      env = {
        "GOMODCACHE" => (work / "go-mod").to_s, "GOPROXY" => "off", "GOSUMDB" => "off",
        "GOWORK" => "off", "GOTOOLCHAIN" => "local", "CGO_ENABLED" => "0",
        "GOFLAGS" => "", "GOOS" => "linux", "GOARCH" => "amd64"
      }
      run "go", "mod", "vendor", chdir: tree, env: env
      capture "nix", "--extra-experimental-features", "nix-command", "hash", "path", tree / "vendor"
    ensure
      # Go module caches contain read-only directories, including newly unpacked modules.
      FileUtils.chmod_R("u+w", temporary)
    end
  end

  def release_metadata(version, cache)
    tag = "v#{version}"
    commit = capture("git", "rev-parse", "--verify", "#{tag}^{commit}")
    date = Time.iso8601(capture("git", "show", "-s", "--format=%cI", commit))
    go_mod = capture("git", "show", "#{tag}:go.mod")
    go_version = go_mod.match(/^go (\S+)$/)[1]
    metadata = {
      "VERSION" => version, "COMMIT" => commit[0, 7], "FULL_COMMIT" => commit,
      "GO_VERSION" => go_version, "GO_MINOR" => go_version.split(".").first(2).join("."),
      "RPM_DATE" => date.strftime("%a %b %d %Y"), "DEBIAN_DATE" => date.rfc2822,
      "MAN_DATE" => date.strftime("%B %Y")
    }
    names = {
      "SOURCE" => "hyprmoncfg-#{version}.tar.gz", "DEPS" => "hyprmoncfg-#{version}-deps.tar.xz",
      "AMD64" => "hyprmoncfg_#{version}_linux_amd64.tar.gz", "ARM64" => "hyprmoncfg_#{version}_linux_arm64.tar.gz"
    }
    base = "#{UPSTREAM}/releases/download/#{tag}"
    download("#{base}/checksums.txt", cache / "checksums.txt")
    names.each do |kind, name|
      url = kind == "SOURCE" ? "#{UPSTREAM}/archive/refs/tags/#{tag}.tar.gz" : "#{base}/#{name}"
      path = cache / name
      download(url, path)
      metadata["#{kind}_URL"] = url
      metadata["#{kind}_NAME"] = name
      metadata["#{kind}_SIZE"] = path.size
      digests(path).each { |algorithm, digest| metadata["#{kind}_#{algorithm.upcase}"] = digest }
      metadata["#{kind}_SRI"] = "sha256-#{[[metadata.fetch("#{kind}_SHA256")].pack('H*')].pack('m0')}"
    end
    verify_assets(cache / "checksums.txt", %w[DEPS AMD64 ARM64].map { |kind| cache / names.fetch(kind) })
    metadata["NIX_VENDOR_SRI"] = nix_vendor_hash(cache / names.fetch("SOURCE"), cache / names.fetch("DEPS"), version)
    metadata
  end

  def generate(output, metadata)
    %w[alpine debian gentoo nix rpm slackware void].each do |directory|
      files(RECIPES / directory).each do |source|
        relative = source.relative_path_from(RECIPES).to_s
        target = output / render(relative, metadata).delete_suffix(".in")
        write(target, render(source.read, metadata), executable: (source.stat.mode & 0o111).positive?)
      end
    end
    { "hyprmoncfg" => %w[SOURCE DEPS], "hyprmoncfg-bin" => %w[AMD64 ARM64] }.each do |name, kinds|
      manifest = kinds.map do |kind|
        "DIST #{metadata.fetch("#{kind}_NAME")} #{metadata.fetch("#{kind}_SIZE")} " \
          "BLAKE2B #{metadata.fetch("#{kind}_BLAKE2B")} SHA512 #{metadata.fetch("#{kind}_SHA512")}\n"
      end.join
      write(output / "gentoo/gui-apps" / name / "Manifest", manifest)
    end
    write(output / "release.json", JSON.pretty_generate(metadata.sort.to_h) + "\n")
  end

  def check(output)
    metadata = JSON.parse((output / "release.json").read)
    Dir.mktmpdir("hyprmoncfg-check-") do |temporary|
      reference = Pathname.new(temporary)
      generate(reference, metadata)
      files(reference).each do |path|
        actual = output / path.relative_path_from(reference)
        raise Error, "generated file is stale: #{actual}" unless actual.file? && actual.binread == path.binread
      end
    end
    files(output).each do |path|
      raise Error, "unexpanded template token in #{path}" if TOKEN.match?(path.read)

      if %w[APKBUILD template].include?(path.basename.to_s) || %w[.ebuild .install .SlackBuild].include?(path.extname)
        run "bash", "-n", path
      end
    end
    puts "Checked all recipes in #{output}"
  end

  def prepare(version, output: nil)
    version = version_arg(version)
    output = (output || ROOT / "dist/packaging" / version).expand_path
    raise Error, "output already exists: #{output}; choose a fresh --output directory" if output.exist?

    %w[git curl go nix bsdtar].each do |command|
      raise Error, "missing required command: #{command}" unless available?(command)
    end
    cache = ROOT / ".cache/packaging" / version
    metadata = release_metadata(version, cache)
    output.dirname.mkpath
    Dir.mktmpdir(".packaging-", output.dirname) do |temporary|
      staging = Pathname.new(temporary) / "recipes"
      generate(staging, metadata)
      check(staging)
      staging.rename(output)
    end
    puts "Prepared #{version}: #{output}\nVerified release assets: #{cache}"
  end

  def artifacts(recipes, output: nil)
    recipes = Pathname.new(recipes).expand_path
    check(recipes)
    metadata = JSON.parse((recipes / "release.json").read)
    output = Pathname.new(output || ROOT / "dist/package-assets").expand_path
    raise Error, "artifact output exists: #{output}" if output.exist?
    output.mkpath
    version = metadata.fetch("VERSION")
    recipe_archive(recipes, output, name: package_name, version: version, epoch: Time.rfc2822(metadata.fetch("DEBIAN_DATE")).to_i)
    # native-packages publishes the AUR recipes as hyprmoncfg-VERSION-packaging.tar.xz
    # with packaging-checksums.txt; keep these source recipes and checksums distinct.
    archive = output / "#{package_name}-#{version}-source-recipes.tar.xz"
    (output / "#{package_name}-#{version}-packaging.tar.xz").rename(archive)
    (output / "packaging-checksums.txt").delete
    write(output / "source-packaging-checksums.txt", "#{sha256(archive)}  #{archive.basename}\n")
    puts "Built release assets in #{output}"
  end

  def main(arguments)
    command = arguments.shift
    return puts(HELP) if %w[-h --help].include?(command)
    raise Error, HELP unless %w[prepare check artifacts].include?(command)

    output = nil
    OptionParser.new do |options|
      options.on("--output DIRECTORY") { |path| output = Pathname.new(path) } if %w[prepare artifacts].include?(command)
    end.parse!(arguments)
    raise Error, HELP unless arguments.length == 1
    NativePackages::Configuration.new(ROOT / "native-packages.yaml").validate

    case command
    when "prepare" then prepare(arguments.first, output: output)
    when "check" then check(Pathname.new(arguments.first).expand_path)
    when "artifacts" then artifacts(arguments.first, output: output)
    end
  end
end

if $PROGRAM_NAME == __FILE__
  begin
    Packages.main(ARGV)
  rescue Packages::Error, SystemCallError, KeyError, ArgumentError, OptionParser::ParseError,
         JSON::ParserError, OpenSSL::OpenSSLError, Psych::Exception, IOError, Timeout::Error => error
    abort "packages: #{error.message}"
  end
end
