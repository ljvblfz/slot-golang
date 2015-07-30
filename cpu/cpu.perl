#!/usr/bin/perl

use warnings;

$SLEEPTIME=5;

if (-e "/tmp/stat") {
unlink "/tmp/stat";
}
open (JIFF_TMP, ">>/tmp/stat") || die "Can't open /proc/stat file!\n";
open (JIFF, "/proc/stat") || die "Can't open /proc/stat file!\n";
@jiff_0=<JIFF>;
print JIFF_TMP $jiff_0[0] ;
close (JIFF);

sleep $SLEEPTIME;

open (JIFF, "/proc/stat") || die "Can't open /proc/stat file!\n";
@jiff_1=<JIFF>;
print JIFF_TMP $jiff_1[0];
close (JIFF);
close (JIFF_TMP);

@USER=`awk '{print \$2}' "/tmp/stat"`;
@NICE=`awk '{print \$3}' "/tmp/stat"`;
@SYSTEM=`awk '{print \$4}' "/tmp/stat"`;
@IDLE=`awk '{print \$5}' "/tmp/stat"`;
@IOWAIT=`awk '{print \$6}' "/tmp/stat"`;
@IRQ=`awk '{print \$7}' "/tmp/stat"`;
@SOFTIRQ=`awk '{print \$8}' "/tmp/stat"`;

$JIFF_0=$USER[0]+$NICE[0]+$SYSTEM[0]+$IDLE[0]+$IOWAIT[0]+$IRQ[0]+$SOFTIRQ[0];
$JIFF_1=$USER[1]+$NICE[1]+$SYSTEM[1]+$IDLE[1]+$IOWAIT[1]+$IRQ[1]+$SOFTIRQ[1];

$SYS_IDLE=($IDLE[0]-$IDLE[1]) / ($JIFF_0-$JIFF_1) * 100;
$SYS_USAGE=100 - $SYS_IDLE;

printf ("The CPU usage is %1.2f%%\n",$SYS_USAGE);
