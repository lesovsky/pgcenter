--
-- Name: pgcenter; Type: SCHEMA; Schema: -
-- The 'IF NOT EXISTS' clause isn't used because it's supported since 9.3.
--

CREATE SCHEMA pgcenter;

--
-- Name: get_netdev_link_settings(character varying); Type: FUNCTION; Schema: pgcenter
--

CREATE FUNCTION pgcenter.get_netdev_link_settings(INOUT iface character varying, OUT speed integer, OUT duplex integer) RETURNS record
    LANGUAGE plperlu
    AS $$
    use Linux::Ethtool::Settings;
    if (my $settings = Linux::Ethtool::Settings->new($_[0])) {
      my $if_speed  = $settings->speed();
      my $if_duplex = $settings->duplex() ? 1 : 0;
      return {iface => $_[0], speed => $if_speed, duplex => $if_duplex};
    } else {
      return {iface => $_[0], speed => 0, duplex => -1};
    }
$$;

--
-- Name: get_sys_clk_ticks(); Type: FUNCTION; Schema: pgcenter
--

CREATE FUNCTION pgcenter.get_sys_clk_ticks() RETURNS integer
    LANGUAGE plperlu
    AS $$
    use POSIX;
    $clock_ticks = POSIX::sysconf( &POSIX::_SC_CLK_TCK );
    return $clock_ticks; 
$$;

--
-- Name: get_proc_stats(character varying, character varying, character varying, integer); Type: FUNCTION; Schema: pgcenter
--

CREATE FUNCTION pgcenter.get_proc_stats(character varying, character varying, character varying, integer) RETURNS SETOF record
    LANGUAGE plperlu
    AS $$
    open FILE, $_[0];
    my @cntn = (); $i = 0;
    while (<FILE>) {
        # skip header if required.
        if ($i < $_[3]) { $i++; next; }
        chomp;
        my @items = map {s/^\s+|\s+$//g; $_;} split ($_[1]);
        my %iitems;
        # use filter if required.
        if ($items[0] =~ $_[2] && $_[2] ne "") {
          @iitems{map 'col'.$_, 0..$#items} = @items;
          push @cntn, \%iitems;
        } elsif ($_[2] eq "") {
          @iitems{map 'col'.$_, 0..$#items} = @items;
          push @cntn, \%iitems;
        }
        $i++
    }
    close FILE;
    return \@cntn; 
$$;

